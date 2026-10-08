# Design decisions

## Dependencies

None. Part 1 uses only the Go standard library (`net/http`, `encoding/json`,
`crypto/rand`, `sync`, …).

## Part 1

### Package layout

```text
cmd/server/            main: flags (-addr, -base), validation, wiring, http.Server.
                       Logic lives in run()/parseConfig() so it is testable; main() only calls run().
internal/shortener/    Domain, no HTTP or storage knowledge: Link type, NormalizeURL,
                       RandomCode, IsValidCode.
internal/store/memory/ In-memory store: two maps + sync.RWMutex.
internal/httpapi/      HTTP handlers and routes on net/http's Go 1.22 ServeMux.
```

Dependency direction: `cmd/server → httpapi → {shortener, memory}`, `memory → shortener`.
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
then: error → HTTP 500 "could not create short link"; nothing is stored
```

With 10 million links stored, a 6-character attempt collides with
probability ≈ 1.7 × 10⁻⁴. Needing all three 6-character attempts to collide
is ≈ 5 × 10⁻¹², so in practice nearly every code is 6 characters. If the
space ever gets crowded, the length grows instead of failing. 
`TestShorten_LengthEscalation` checks the exact attempt sequence
`6,6,6,7,7,7,8,8,8` and the final error.

Redirect requests for paths that cannot be a code (not 6–8 base62 chars,
e.g. `/favicon.ico`) get **404** without touching the store.

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

Part 3 benchmarks will confirm or revisit this choice.

### HTTP details

- **Request body:** limited to 8 KiB (`http.MaxBytesReader`). It must be
  exactly one JSON object. Malformed JSON, a non-string `url`, or trailing
  data → **400**. `Content-Type` is not enforced, so
  `curl -d '{"url":…}'` without the header also works.
- **Errors:** responses are JSON `{"error":"<message>"}` with 400/404/500.
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
