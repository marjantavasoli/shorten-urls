# Design decisions

## Dependencies

None. Part 1 uses only the Go standard library (`net/http`, `encoding/json`,
`crypto/rand`, `sync`, …).

## Part 1

### Package layout

```text
cmd/server/            main: flags (-addr, -base, -max-links), validation, wiring, http.Server.
                       Logic lives in run()/parseConfig() so it is testable; main() only calls run().
internal/shortener/    Domain, no HTTP or storage knowledge: Link type, NormalizeURL,
                       RandomCode, IsValidCode, sentinel errors (Part 2).
internal/store/memory/ In-memory store: two maps + sync.RWMutex.
internal/httpapi/      HTTP handlers and routes on net/http's Go 1.22 ServeMux,
                       plus the Store interface they depend on (Part 2).
```

Dependency direction: `cmd/server → {httpapi, memory}`, `httpapi → shortener`,
`memory → shortener`. Since Part 2, `httpapi` no longer imports `memory`:
`cmd/server` is the only place that knows which store is used.
Nothing imports `httpapi` or `cmd`.

- **Router:** standard `net/http.ServeMux` with method + wildcard patterns
  (`POST /api/shorten`, `GET /{code}`). No third-party router is needed.
  A `GET` pattern also matches `HEAD`, so `curl -I /<code>` returns the 302,
  and wrong methods get an automatic **405** with an `Allow` header.

### When two URLs are "the same" (normalization)

`shortener.NormalizeURL` produces a canonical string. Two URLs are the same
if and only if their canonical forms are byte-identical. Only
transformations that cannot change the target resource are applied
(RFC 3986 §6.2.2–6.2.3):

| Rule | Example | Why |
|---|---|---|
| Trim surrounding whitespace | `" https://a.com/ "` → `https://a.com/` | Copy-paste noise |
| Lowercase scheme and host | `HTTPS://Go.DEV/doc` → `https://go.dev/doc` | Case-insensitive by spec |
| Drop the scheme's default port | `http://a.com:80/x` → `http://a.com/x` | Same endpoint |
| Empty path → `/` | `https://a.com` → `https://a.com/` | Same resource |
| **Keep** path case | `/Doc` ≠ `/doc` | Paths are case-sensitive |
| **Keep** trailing slash on non-root paths | `/doc` ≠ `/doc/` | Servers may serve different resources |
| **Keep** query order and fragment | `?b=1&a=2` unchanged | Order can matter; fragment is part of what the user shared |

Being conservative means two equivalent-but-differently-written URLs may get
two codes. Both still work. The opposite mistake, merging two URLs that are
actually different, would send users to the wrong page, so it is avoided.

Validation (any failure → **400**):
- non-empty after trimming
- at most 2048 bytes
- parses with `net/url`
- scheme is `http` or `https` (case-insensitive), which rejects `javascript:`, `data:`, `ftp:`, `mailto:`
- absolute, with a non-empty host (rejects relative URLs, `http:example.com`, `http:///x`)

The long URL is **only parsed, never fetched**, so there is no SSRF surface.
The **canonical form is what is stored and what is sent in `Location`**.

### How "same URL → same code" is stored

The store keeps two maps:

```go
byCode map[string]shortener.Link // code → link      (redirect lookups)
byURL  map[string]string         // canonical URL → code (idempotency index)
```

Both are updated together under the write lock, so they never disagree.
`POST /api/shorten` returns **201** for both new and existing URLs, as the
spec requires.

The store treats its input as an opaque key; normalizing before calling it
is the handler's job.

### How codes are generated (new URLs only)

- `shortener.RandomCode(n)`: for each of the `n` characters, pick an index
  in `[0, 62)` with `crypto/rand.Int(rand.Reader, big.NewInt(62))` and map it
  to base62 `[A-Za-z0-9]`. `rand.Int` is uniform (it rejects out-of-range
  samples internally), so there is no modulo bias.
  `math/rand/v2` is not used because its documentation says it is not for
  security-sensitive work, and unguessable codes are the point.
- **Length 6–8, growing on collisions:** a code starts at 6 characters
  (62⁶ ≈ 5.7 × 10¹⁰ codes); see collision handling below.
- The generator is called **only when the URL is not in `byURL`**. A test
  asserts it runs exactly once for three shortens of the same URL.
- **Why random, not a counter + base62:**
  - Counter codes are sequential, so anyone could enumerate every stored link.
  - A counter needs coordination across replicas (Part 5) and a DB sequence (Part 4).
  - Random codes with a uniqueness check carry over directly to Postgres as a
    `UNIQUE` constraint plus the same retry loop.
- The generator is injectable (`memory.WithCodeFunc`) so tests can force
  collisions deterministically.

### Collision handling

Distinct URLs must never share a code, so a candidate is inserted only if
`byCode` does not already contain it, checked under the write lock.

On a collision, the store retries with a fresh random code, escalating the length:

```text
length 6: up to 3 attempts
length 7: up to 3 attempts   (62⁷ ≈ 3.5 × 10¹²)
length 8: up to 3 attempts   (62⁸ ≈ 2.2 × 10¹⁴)
then: error → HTTP 500 {"error":"internal error"}; nothing is stored
```

With 10 million links stored, a 6-character attempt collides with
probability ≈ 1.7 × 10⁻⁴. Needing all three 6-character attempts to collide
is ≈ 5 × 10⁻¹², so in practice nearly every code is 6 characters. If the
space ever gets crowded, the length grows instead of failing. 
`TestShorten_LengthEscalation` checks the exact attempt sequence
`6,6,6,7,7,7,8,8,8` and the final error.

Requests for paths that cannot be a code (not 6–8 base62 chars, e.g.
`/favicon.ico`) get **404** without touching the store. Since Part 2 this
applies to both the redirect and the metadata route.

### Mutex type and which methods lock

`sync.RWMutex`, because redirects are expected to far outnumber creates.
Readers run in parallel and only creating a new link takes the exclusive lock.

| Method | Lock |
|---|---|
| `Get(code)` (redirect) | `RLock` |
| `Len()` | `RLock` |
| `Shorten(url)`, URL already known (fast path) | `RLock` only |
| `Shorten(url)`, new URL | `Lock`, held for the re-check + collision check + insert |

For a new URL, the random code is generated **before** taking `Lock`
(crypto/rand needs no shared state), which keeps the critical section to a
few map operations. Under `Lock` the store **re-checks** `byURL`, because
another goroutine may have inserted the same URL between the read-locked
fast path and the write lock. If it did, that existing link is returned and
the candidate is discarded. This check-then-insert under one lock makes
concurrent duplicate shortens converge on one code
(`TestShorten_ConcurrentSameURL`, 200 goroutines released at once; also over
HTTP in `TestConcurrentShortenSameURLAndRedirect`).

Part 3 measured this choice; the `RWMutex` stays (see Part 3, "Locking choice").

### HTTP details

- **Request body:** limited to 8 KiB (`http.MaxBytesReader`). It must be
  exactly one JSON object. Malformed JSON, a non-string `url`, or trailing
  data → **400**. `Content-Type` is not enforced, so
  `curl -d '{"url":…}'` without the header also works.
- **Errors:** responses are JSON `{"error":"<message>"}` with 400/404/500.
  Part 2 describes how errors are mapped to these.
- **Redirect:** `Location` is set directly and the handler writes **302**
  with no body. `http.Redirect` is not used because it can rewrite the target
  and adds an HTML body.
- **`-base`:**
  - Must be an absolute `http(s)` URL with a host and no query or fragment;
    otherwise the server refuses to start.
  - A trailing slash is stripped.
  - A path prefix is allowed (e.g. behind a proxy).
  - `short_url = base + "/" + code`.
- **`-addr`:** listen address, default `:8080`.
- **Logging:** if a create fails, the error is logged but **never the long
  URL**, since query strings may contain secrets.

## Part 2

### Sentinel errors

Defined in the domain package, `internal/shortener/errors.go`:

```go
var (
	ErrInvalidURL = errors.New("invalid url")
	ErrNotFound   = errors.New("link not found")
)
```

- **Why in `shortener` and not in a store or HTTP package:** every store
  (memory now, Postgres in Part 4) returns them and `httpapi` checks them.
  Putting them in the shared domain package means none of those packages
  has to import another.
- `NormalizeURL` wraps `ErrInvalidURL` with the specific reason:
  `fmt.Errorf("%w: url scheme must be http or https", ErrInvalidURL)`.
  `err.Error()` is then `"invalid url: url scheme must be http or https"`,
  and `errors.Is(err, ErrInvalidURL)` is true.
- `memory.Store.Get` returns `fmt.Errorf("get %q: %w", code, ErrNotFound)`
  for an unknown code.
- Validation errors come from the domain (`NormalizeURL`), not the store.
  The handler normalizes before calling the store, so stores never see
  invalid input.

### `Store` interface: location and methods

Defined in `internal/httpapi/store.go`, the package that uses it:

```go
type Store interface {
	Shorten(longURL string) (shortener.Link, error)
	Get(code string) (shortener.Link, error)
}
```

- **Location:** at the consumer, following "accept interfaces, return
  structs". `memory.Store` satisfies it without importing `httpapi`. The
  interface lists only the two methods the handlers call (`Len()` stays on
  `memory.Store` as a test helper). `cmd/server` is the only place a
  concrete store is chosen. Passing `memory.New()` to `httpapi.New`, which
  takes a `Store`, is itself the compile-time check that the memory store
  satisfies the interface, so no separate `var _ httpapi.Store = …`
  assertion is kept.
- **`Shorten(longURL)`** expects an already-normalized URL and returns the
  existing link or creates one. Same URL → same code: the Part 1
  idempotency rule is part of the interface contract, not just a property
  of the memory implementation.
- **`Get(code)`** changed from `(Link, bool)` in Part 1 to `(Link, error)`.
  A `bool` can only say "missing". A database-backed store (Part 4) can also
  fail for other reasons (connection lost, timeout), and those must become
  500, not 404. "Not found" is now an error wrapping `ErrNotFound`.
- **No `context.Context` yet.** The memory store does no I/O and has
  nothing to cancel, so a `ctx` parameter would go unused. It will be added
  in Part 4, where Postgres queries need cancellation and deadlines
  (`db.WithContext(ctx)`).

### How errors become status codes and response bodies

All handlers send domain and store errors through one function,
`writeDomainError`, which checks them with `errors.Is`. It finds the
sentinel however many times the error has been wrapped with `%w`.

| Error | Status | Body |
|---|---|---|
| wraps `ErrInvalidURL` | 400 | `{"error":"invalid url: <reason>"}`: the reason is safe and helps the client fix the request |
| wraps `ErrNotFound` | 404 | `{"error":"not found"}` |
| wraps `ErrStoreFull` (added in Part 3) | 507 | `{"error":"link storage is full"}` |
| anything else | 500 | `{"error":"internal error"}`: details are logged on the server and never sent to the client |

Request-format errors are found while decoding the body, before any domain
call, and return 400 directly:
- malformed JSON or a non-string `url`
- trailing data after the JSON object
- a body larger than 8 KiB

A body that does not arrive before `ReadTimeout` returns **408** (added in
Part 3). Wrong methods get 405 from `ServeMux`.

The 500 body is deliberately generic. A store error may contain hostnames,
SQL or file paths (Part 4), and the client can't act on any of that.

### Metadata route

`GET /api/v1/links/{code}` → **200** `{"url":"…","created_at":"…"}`, or
**404** `{"error":"not found"}` for unknown or malformed codes.

- **`created_at` format:** RFC 3339 in UTC with whole seconds, e.g.
  `2026-01-15T12:00:00Z`, matching the spec's example. The response struct
  holds a pre-formatted string, `CreatedAt.UTC().Format(time.RFC3339)`.
  Marshalling a `time.Time` directly would use RFC3339**Nano**
  (`2026-01-15T12:00:00.123456789Z`) and keep the original time zone
  offset. The stored value keeps full precision; only the API output is
  truncated to seconds.
- The response contains exactly `url` and `created_at`, as specified.
- It shares the `lookup` helper with the redirect route (code-shape check,
  then `Store.Get`), so both routes behave the same for unknown and
  malformed codes.
- There is no route conflict with `GET /{code}`, because the two patterns
  have different numbers of path segments.

### Tests

- **`fakeStore`** (`internal/httpapi/fake_store_test.go`) has injectable
  `ShortenFn`/`GetFn` and records its arguments. It is used to check:
  - error mapping for `ErrNotFound` → 404, an unknown error → 500 with no
    details leaked, and a doubly-wrapped `ErrNotFound` → 404;
  - that the handler passes the **normalized** URL to the store;
  - that the store is **never called** for invalid URLs or malformed codes;
  - the redirect and metadata responses produced from a stored link.
- **Metadata route against the real memory store:** a fixed clock in a
  non-UTC zone with nanoseconds checks the exact JSON (`created_at` in UTC,
  no fraction, exactly two keys), that no `Location` header is set, and the
  404 cases.
- **Idempotency after the refactor:** the Part 1 HTTP tests
  (`TestShorten_Idempotent`, `TestConcurrentShortenSameURLAndRedirect`) now
  run through the `Store` interface, because `httpapi.New` takes a `Store`.
  The memory store's idempotency and concurrency tests are unchanged.

## Part 3

Numbers below come from Go 1.24.7, linux/amd64, on an Intel Xeon @ 2.10GHz
with **2 vCPUs**. They are relative measurements on one machine, not
production capacity.

### Locking choice: `sync.RWMutex` (kept, now measured)

Part 1 chose `RWMutex` because redirects should far outnumber creates. Part 3
checks that with `BenchmarkLock_ReadHeavy`
(`internal/store/memory/store_bench_test.go`):
- the store's access pattern in isolation: a `map[string]Link` with 100k entries;
- 99% reads and 1% inserts;
- run with `b.RunParallel`;
- the same code with each lock type.

A small adapter makes a plain `Mutex` satisfy the same `Lock/RLock`
interface, so both variants pay the same interface-call cost and differ only
in the lock.

`go test -run "^$" -bench Lock_ReadHeavy -cpu 1,4,8 -count 3 ./internal/store/memory/`
(medians, ns/op, lower is better):

| goroutines (`-cpu`) | `Mutex` | `RWMutex` | |
|---|---|---|---|
| 1 | 54.6 | 56.4 | tie (within noise) |
| 4 | 99.0 | 66.6 | `RWMutex` ~33% faster |
| 8 | 116.6 | 71.4 | `RWMutex` ~39% faster |

With one goroutine there is no contention, so the two locks cost the same.
As soon as several goroutines read at once, `Mutex` serializes the readers,
while `RWMutex` lets them proceed together. The gap grows with concurrency,
and a server handling many redirects is exactly that case. **Decision: keep
`RWMutex`.** The lock table from Part 1 (which methods take `RLock` vs
`Lock`) is unchanged.

Caveat: with 2 vCPUs, `-cpu 4,8` means more goroutines than cores. That
measures contention, not true 8-core parallelism. The direction of the
result is the standard one for read-heavy maps; on more cores the gap is
expected to stay or widen.

The production store does not use the adapter. It keeps a concrete
`sync.RWMutex` field, so the hot path has no interface-call overhead. The
comparison lives only in the benchmark file. That was a deliberate change
from my first plan, which would have made the store's lock pluggable.

### Timeout values and slow-client behavior

Set on `http.Server` in `cmd/server` (`newServer`). They are fields on the
`config` struct with defaults in `defaultConfig()`, **not CLI flags**, so
tests can shorten them while operators get one sensible set:

| Field | Value | Why this value |
|---|---|---|
| `ReadHeaderTimeout` | 5s | Headers are a few hundred bytes; any real client sends them in well under a second |
| `ReadTimeout` | 10s | Covers headers plus a body of at most 8 KiB, even on a poor mobile link |
| `WriteTimeout` | 10s | Responses are under 200 bytes |
| `IdleTimeout` | 60s | Keep-alive reuse for normal browsing, without holding idle sockets forever |
| `MaxHeaderBytes` | 16 KiB | The default is 1 MiB; this API needs a fraction of that |

What a slow or misbehaving client experiences:

| Client behavior | Result | Tested by |
|---|---|---|
| Sends headers byte by byte (slowloris) | Connection closed after 5s with no response, since no request was ever parsed. Frees the goroutine and socket | `TestServer_SlowHeadersAreCutOff` (real TCP socket, timeouts shortened to ~150 ms) |
| Sends headers, then the body too slowly | **408** `{"error":"request body was not received in time"}`. The handler detects a `net.Error` with `Timeout()` from the body read; without this check, the timeout would have been reported as a misleading 400 "must be a JSON object" | `TestServer_SlowBodyGets408` (real socket), `TestShorten_BodyReadTimeoutIs408` (unit) |
| Stops reading the response | The write fails after 10s and the connection is closed | (standard library behavior) |
| Keeps an idle keep-alive connection | Closed after 60s | (standard library behavior) |

`http.TimeoutHandler` is not used. Our handlers do no slow work: they spend
microseconds in a map. Slow server-side work arrives with Postgres in Part 4
and will be bounded through `context` deadlines on the queries.

### Eviction cap: cap without eviction (`-max-links`, default 1,000,000)

- **Measured cost per link: about 253 bytes of heap**
  (`BenchmarkMemoryPerLink`: 100k links with ~80-character URLs, heap
  measured after `runtime.GC()`). This covers both maps, the `Link` value,
  the code string, the URL string (shared between `byURL`'s key and
  `Link.URL`) and map overhead. Longer URLs cost more, roughly one byte per
  extra character.
- **Default cap 1,000,000 ≈ 250 MB of live heap.** With Go's default
  `GOGC=100`, peak heap can reach about twice the live heap, so roughly
  500 MB. That is a safe ceiling for a small VM or container.
  `-max-links 0` disables the cap.
- **When full:**
  - **existing** URLs still return **201** with their code, because the
    idempotent fast path is checked before the cap;
  - redirects and metadata keep working;
  - only **new** URLs are rejected, with **507 Insufficient Storage**
    `{"error":"link storage is full"}` (`shortener.ErrStoreFull`, mapped in
    `writeDomainError`).
- **Why no eviction:** evicting a link means a short URL someone already
  shared starts returning 404. For a URL shortener that is a correctness
  bug, not a cache miss. Refusing new links is visible and recoverable;
  silently breaking old ones is neither.
- **The check is atomic:** it runs under the same write lock as the insert,
  so concurrent creates can never overshoot the cap
  (`TestShorten_ConcurrentAtCapacity`: 100 goroutines, cap 10 → exactly 10
  links).
- **The cap also bounds GC cost.** Profiling (below) shows that GC marking
  is the largest CPU cost on the create path, and it grows with the number
  of stored links.
- In Part 4, the durable Postgres store has no in-process cap; database
  capacity is managed separately.

### Benchmarks

| Package | Benchmark | What it isolates |
|---|---|---|
| `memory` | `Shorten_New`, `Shorten_Existing` | Create path (code generation + write lock) vs idempotent fast path (read lock) |
| `memory` | `Get`, `Get_Parallel`, `Mixed_Parallel` | Redirect lookup alone, under contention, and with a realistic 99:1 read/write mix |
| `memory` | `Lock_ReadHeavy/{Mutex,RWMutex}`, `MemoryPerLink` | The locking decision and the eviction-cap sizing above |
| `httpapi` | `HTTP_Shorten_New`, `HTTP_Shorten_Existing`, `HTTP_Redirect`, `HTTP_Metadata` | Full handler path via `ServeHTTP` (routing, JSON, store), without TCP |
| `shortener` | `RandomCode`, `NormalizeURL`, `IsValidCode` | Building blocks of the create and redirect paths |

All use `b.ReportAllocs()` and the classic `for i := 0; i < b.N; i++` loop,
because `b.Loop()` needs Go 1.24 and the module declares 1.22.

### Profiling insight

The README shows the commands and the main finding. In short:

- **GC is the largest single cost on the create path.** About 30% of CPU
  samples are background GC marking (`runtime.gcBgMarkWorker` →
  `scanobject`), and allocation itself (`runtime.mallocgc`) adds about 21%.
  Both are more than any single function of our own. The store keeps every link, and every link holds
  pointers (strings), so each GC cycle must scan the whole store. That cost
  grows with the number of links, which is another reason for the cap.
- **Of our own code:**
  - `store.Shorten` ≈ 17% of samples, about half of it `crypto/rand.Int`;
  - JSON decoding ≈ 12%;
  - JSON encoding of the response ≈ 10%.

  `RandomCode` costs ~1 µs and 20 allocations per 6-character code, because
  `rand.Int` allocates `big.Int` values for every character. That is the
  price of the simpler implementation chosen in Part 1. It only affects
  creates, which are rare compared with redirects, so it is left as is.
  Filling a byte buffer with a single `rand.Read` would cut it to about one
  allocation if creates ever become hot.
- **The redirect path is cheap:** about 1 µs end to end through
  `ServeHTTP`, with a 0-allocation, ~60 ns store lookup.
- **Harness overhead:** the B/op of the HTTP benchmarks is inflated by
  `httptest.NewRequest`, which allocates a 4 KiB `bufio.Reader` per request
  (51% of allocated bytes in the memory profile). That is test scaffolding,
  not server cost; a real `http.Server` reuses per-connection buffers.
