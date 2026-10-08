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

Links are kept in memory (Part 1) and are lost on restart.

## API

| Method | Path | Success | Errors |
|---|---|---|---|
| `POST` | `/api/shorten` | **201** `{"code","short_url"}`. The same URL always returns the same code | **400** missing, empty or invalid `url` (only `http`/`https`), bad JSON |
| `GET` / `HEAD` | `/{code}` | **302** with `Location: <long url>` | **404** unknown code |

Error bodies are JSON: `{"error":"..."}`.

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
```

## Test

```bash
gofmt -l .                 # prints nothing
go vet ./...
go test ./...
go test -race ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -n1
```

Current coverage:

```text
total:							(statements)	95.5%
```
