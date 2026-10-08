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

| Done | Pts | Requirement |
|:----:|:---:|-------------|
| [ ] | 5 | Metadata route **200** / **404** with correct JSON |
| [ ] | 4 | `ErrNotFound`, `ErrInvalidURL` from store/domain |
| [ ] | 4 | `%w` + `errors.Is` in HTTP mapping |
| [ ] | 5 | `Store` interface + fake used in tests |
| [ ] | 4 | Tests for metadata route and error mapping |
| [ ] | 3 | Test or note in README: idempotency still works via `Store` / HTTP after Part 2 changes |

## Part 3 — Performance & measurement (25 points)

| Done | Pts | Requirement |
|:----:|:---:|-------------|
| [ ] | 5 | Server timeouts configured |
| [ ] | 5 | `Mutex` vs `RWMutex` matches behavior and `DECISIONS.md` |
| [ ] | 5 | Benchmarks for shorten and redirect |
| [ ] | 5 | README: benchmark line + profiling insight |
| [ ] | 5 | Tests and `-race` still green |

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
