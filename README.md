# URL shortener

A small HTTP service that shortens long URLs and redirects short codes to the
original link. It is written in Go 1.22+ using only the standard library.

Design choices are documented in [`DECISIONS.md`](DECISIONS.md), and progress
is tracked in [`CHECKLIST.md`](CHECKLIST.md).

## Run

```bash
go run ./cmd/server -addr :8080 -base http://localhost:8080
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8080` | Listen address |
| `-base` | `http://localhost:8080` | Public base URL used to build `short_url`. Must be an absolute `http(s)` URL; a trailing slash is ignored |
| `-max-links` | `1000000` | Maximum links kept in memory (~253 B each, ≈ 250 MB at the default). When full, **new** URLs get **507**; existing URLs, redirects and metadata keep working. `0` = unlimited |

Links are kept in memory and are lost on restart.

Server timeouts are fixed in code (not flags): read header 5s, read 10s,
write 10s, idle 60s, max header size 16 KiB. See `DECISIONS.md` → Part 3.

## API

| Method | Path | Success | Errors |
|---|---|---|---|
| `POST` | `/api/shorten` | **201** `{"code","short_url"}`. The same URL always returns the same code | **400** missing, empty or invalid `url` (only `http`/`https`), bad JSON; **408** body not received in time; **507** store full (new URLs only) |
| `GET` / `HEAD` | `/{code}` | **302** with `Location: <long url>` | **404** unknown code |
| `GET` | `/api/v1/links/{code}` | **200** `{"url","created_at"}` (RFC 3339, UTC), no redirect | **404** unknown code |

Error bodies are JSON: `{"error":"..."}`. Any unexpected server-side failure
returns **500** `{"error":"internal error"}`; the details are only logged.

### Example

```bash
curl -s -X POST localhost:8080/api/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/"}'
# {"code":"WoWswQ","short_url":"http://localhost:8080/WoWswQ"}

curl -sI localhost:8080/WoWswQ
# HTTP/1.1 302 Found
# Location: https://go.dev/doc/

# Same URL again → same code (still 201)
curl -s -X POST localhost:8080/api/shorten \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://go.dev/doc/"}'
# {"code":"WoWswQ","short_url":"http://localhost:8080/WoWswQ"}

curl -s -X POST localhost:8080/api/shorten -d '{"url":"ftp://x.com"}'
# {"error":"invalid url: url scheme must be http or https"}   (400)

curl -s localhost:8080/zzzzzz
# {"error":"not found"}   (404)

# Metadata without redirecting
curl -s localhost:8080/api/v1/links/WoWswQ
# {"url":"https://go.dev/doc/","created_at":"2026-10-08T16:11:49Z"}

curl -s localhost:8080/api/v1/links/zzzzzz
# {"error":"not found"}   (404)
```

### Idempotency after the Part 2 refactor

Since Part 2, the handlers depend on the `httpapi.Store` interface instead
of the concrete memory store. Same URL → same code still holds, and these
tests check it through the interface:

- `internal/httpapi`: `TestShorten_Idempotent` (sequential, including
  normalized-equivalent URLs) and `TestConcurrentShortenSameURLAndRedirect`
  (100 concurrent requests for one URL → one code) run over HTTP against the
  memory store passed in as a `Store`.
- `internal/store/memory`: `TestShorten_SameURLSameCode` and
  `TestShorten_ConcurrentSameURL` (200 goroutines).

## Performance

All commands here use `-flag value` (a space, not `=`) so they work
unchanged in bash, PowerShell and cmd. PowerShell splits `-bench=.` into
`-bench=` and `.`, which makes Go treat `.` as a package and fail with
`no Go files in ...`.

Run all benchmarks:

```bash
go test -run "^$" -bench . -benchmem ./...
```

Measured on Go 1.24.7, linux/amd64, Intel Xeon @ 2.10GHz, 2 vCPUs.

**Benchmark line (redirect, full handler path):**

```text
BenchmarkHTTP_Redirect-2   	 1217385	       961.0 ns/op	     960 B/op	       9 allocs/op
```

The store lookup inside it is `BenchmarkGet-2   59.30 ns/op   0 B/op   0 allocs/op`.
The rest of the ~1 µs is routing, the response recorder and headers.

**Profiling insight:**

```bash
go test -run "^$" -bench HTTP_Shorten_New -benchtime 3s -cpuprofile cpu.out -memprofile mem.out -o httpapi.test ./internal/httpapi/
go tool pprof -top -cum httpapi.test cpu.out
go tool pprof -top -sample_index=alloc_space httpapi.test mem.out
```

- **The garbage collector, not our code, is the largest cost of creating
  links.** About 30% of CPU samples are background GC marking
  (`runtime.gcBgMarkWorker` → `scanobject`), and allocation
  (`runtime.mallocgc`) adds about 21%. The store keeps every link, and every
  link contains pointers, so each GC cycle scans the whole store. That work
  grows with the number of links, which is one reason for the `-max-links`
  cap.
- **Within our code:**
  - `store.Shorten` ≈ 17%, about half of it `crypto/rand.Int`. `RandomCode`
    costs ~1 µs and 20 allocs per code, because `rand.Int` allocates
    `big.Int` values per character. That is acceptable because creates are
    rare compared with redirects.
  - JSON decoding ≈ 12%.
  - JSON encoding ≈ 10%.
- **Harness overhead:** half of the allocated bytes in `HTTP_*` benchmarks
  come from `httptest.NewRequest` (a 4 KiB `bufio.Reader` per request),
  which a real server doesn't pay per request.

**Locking:** `RWMutex` vs `Mutex` on a 99%-read workload: tie with one
goroutine, and `RWMutex` 33–39% faster with 4–8 concurrent goroutines.
Details are in `DECISIONS.md` → Part 3.

## Test

```bash
gofmt -l .                 # prints nothing
go vet ./...
go test ./...
go test -race ./...
go test -coverprofile coverage.out ./...
go tool cover -func coverage.out | tail -n1          # PowerShell: | Select-Object -Last 1
```

Current coverage:

```text
total:							(statements)	96.2%
```
