# Checklist

## Part 1 — MVP (25 points)

| Done | Pts | Requirement | Where |
|:----:|:---:|-------------|-------|
| [x] | 4 | `POST /api/shorten` returns **201** with `code` and `short_url` | `httpapi.TestShortenAndRedirect` |
| [x] | 3 | **Idempotency:** second `POST` with the **same URL** returns the **same** `code` (test required) | `httpapi.TestShorten_Idempotent`, `memory.TestShorten_SameURLSameCode` |
| [x] | 4 | `GET /{code}` returns **302** with correct `Location` | `httpapi.TestShortenAndRedirect` (GET and HEAD) |
| [x] | 3 | Unknown code → **404**; bad/missing URL → **400** | `httpapi.TestRedirect_NotFound`, `httpapi.TestShorten_BadRequest` |
| [x] | 3 | URL validation and no server-side fetch of long URL | `shortener.NormalizeURL` + tests; parsed only, never fetched |
| [x] | 2 | Codes 6–8 chars for new URLs; collision strategy for **new** codes only | `memory.TestShorten_LengthEscalation`, `TestShorten_CollisionRetry`, `TestShorten_ExistingURLSkipsGeneration` |
| [x] | 2 | `-base` flag used for `short_url` | `cmd/server` tests, `httpapi.TestShorten_BaseTrailingSlash` |
| [x] | 2 | `httptest`: shorten + redirect; table tests for bad URL and unknown code | `internal/httpapi/handler_test.go` |
| [x] | 2 | Concurrent test (include concurrent duplicate shorten for same URL); **`go test -race ./...`** passes | `memory.TestShorten_ConcurrentSameURL`, `httpapi.TestConcurrentShortenSameURLAndRedirect` |

## Part 2 — API & errors (25 points)

| Done | Pts | Requirement | Where |
|:----:|:---:|-------------|-------|
| [x] | 5 | Metadata route **200** / **404** with correct JSON | `httpapi.TestMetadata_OK`, `TestMetadata_NotFound`, `TestRedirectAndMetadata_WithFake` |
| [x] | 4 | `ErrNotFound`, `ErrInvalidURL` from store/domain | `internal/shortener/errors.go`; `shortener.TestNormalizeURL_Invalid`, `memory.TestGet_Unknown` |
| [x] | 4 | `%w` + `errors.Is` in HTTP mapping | `httpapi.writeDomainError`; `httpapi.TestWriteDomainError` (incl. doubly-wrapped) |
| [x] | 5 | `Store` interface + fake used in tests | `internal/httpapi/store.go`, `internal/httpapi/fake_store_test.go` |
| [x] | 4 | Tests for metadata route and error mapping | `internal/httpapi/metadata_test.go`, `internal/httpapi/errors_test.go` |
| [x] | 3 | Test or note in README: idempotency still works via `Store` / HTTP after Part 2 changes | README "Idempotency after the Part 2 refactor"; `httpapi.TestShorten_Idempotent`, `TestConcurrentShortenSameURLAndRedirect` |

## Part 3 — Performance & measurement (25 points)

| Done | Pts | Requirement | Where |
|:----:|:---:|-------------|-------|
| [x] | 5 | Server timeouts configured | `cmd/server.newServer`; `TestNewServer_Timeouts`, `TestServer_SlowHeadersAreCutOff`, `TestServer_SlowBodyGets408` |
| [x] | 5 | `Mutex` vs `RWMutex` matches behavior and `DECISIONS.md` | `RWMutex` in `memory.Store`; measured by `BenchmarkLock_ReadHeavy`; DECISIONS → Part 3 |
| [x] | 5 | Benchmarks for shorten and redirect | `memory/store_bench_test.go`, `httpapi/handler_bench_test.go`, `shortener/bench_test.go` |
| [x] | 5 | README: benchmark line + profiling insight | README → Performance |
| [x] | 5 | Tests and `-race` still green | `go test -race ./...` |

## Part 4 — Persistence (25 points)

| Done | Pts | Requirement |
|:----:|:---:|-------------|
| [ ] | 6 | Persistent `Store` (GORM+DB or file-backed) |
| [ ] | 5 | Startup load |
| [ ] | 5 | Create persisted before response |
| [ ] | 4 | Restart test (temp DB or temp files) |
| [ ] | 3 | Config selects memory vs persistent store |
| [ ] | 2 | `-race` clean with persistent store |

## Part 5 — Millions of requests (optional, +10 max)

| Done | Pts | Requirement |
|:----:|:---:|-------------|
| [ ] | 4 | `DECISIONS.md`: LB → N apps → shared store |
| [ ] | 3 | CDN / edge caching for redirects |
| [ ] | 3 | Write-path scaling (rate limit, queue, or code pool) |
| [ ] | 3 | Sharding / partitioning strategy |
| [ ] | 4 | **Bonus code:** cache layer, load-test script + README numbers, etc. |

## Part 6 — Production habits (optional, +10 max)

| Done | Pts | Requirement |
|:----:|:---:|-------------|
| [ ] | 3 | Graceful shutdown working + documented |
| [ ] | 3 | Rate limit on create |
| [ ] | 2 | Domain policy in `DECISIONS.md` |
| [ ] | 2 | What you log vs never log |
| [ ] | 4 | **Bonus:** pprof/metrics behind flag, or structured logging |
