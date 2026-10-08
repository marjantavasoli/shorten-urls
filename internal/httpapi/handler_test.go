package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"url-shortener/internal/shortener"
	"url-shortener/internal/store/memory"
)

const testBase = "http://sho.rt"

func newTestHandler(opts ...memory.Option) *Handler {
	return New(memory.New(opts...), testBase)
}

func doShorten(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeShorten(t *testing.T, rec *httptest.ResponseRecorder) shortenResponse {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var resp shortenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestShortenAndRedirect(t *testing.T) {
	h := newTestHandler()

	resp := decodeShorten(t, doShorten(t, h, `{"url":"https://go.dev/doc/"}`))
	if !shortener.IsValidCode(resp.Code) {
		t.Errorf("code %q is not 6-8 base62 chars", resp.Code)
	}
	if want := testBase + "/" + resp.Code; resp.ShortURL != want {
		t.Errorf("short_url = %q, want %q", resp.ShortURL, want)
	}

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/"+resp.Code, nil))
		if rec.Code != http.StatusFound {
			t.Errorf("%s: status = %d, want 302", method, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "https://go.dev/doc/" {
			t.Errorf("%s: Location = %q", method, loc)
		}
	}
}

func TestShorten_Idempotent(t *testing.T) {
	h := newTestHandler()
	first := decodeShorten(t, doShorten(t, h, `{"url":"https://go.dev/doc/"}`))
	second := decodeShorten(t, doShorten(t, h, `{"url":"https://go.dev/doc/"}`))
	if first != second {
		t.Errorf("same URL: first %+v, second %+v", first, second)
	}
	third := decodeShorten(t, doShorten(t, h, `{"url":"HTTPS://GO.DEV:443/doc/"}`))
	if third != first {
		t.Errorf("normalized-equivalent URL: got %+v, want %+v", third, first)
	}
	other := decodeShorten(t, doShorten(t, h, `{"url":"https://go.dev/doc"}`))
	if other.Code == first.Code {
		t.Error("/doc and /doc/ must not share a code")
	}
}

func TestShorten_BaseTrailingSlash(t *testing.T) {
	h := New(memory.New(), "https://sho.rt/")
	resp := decodeShorten(t, doShorten(t, h, `{"url":"https://example.com/"}`))
	if want := "https://sho.rt/" + resp.Code; resp.ShortURL != want {
		t.Errorf("short_url = %q, want %q", resp.ShortURL, want)
	}
}

func TestShorten_BadRequest(t *testing.T) {
	tests := []struct {
		name, body, wantMsg string
	}{
		{"missing url", `{}`, "url is required"},
		{"empty url", `{"url":""}`, "url is required"},
		{"blank url", `{"url":"   "}`, "url is required"},
		{"null body", `null`, "url is required"},
		{"ftp scheme", `{"url":"ftp://example.com/f"}`, "http or https"},
		{"javascript scheme", `{"url":"javascript:alert(1)"}`, "http or https"},
		{"no scheme", `{"url":"example.com"}`, "http or https"},
		{"relative", `{"url":"/path"}`, "http or https"},
		{"no host", `{"url":"http:///x"}`, "host"},
		{"url not a string", `{"url":123}`, "JSON object"},
		{"malformed json", `{"url":`, "JSON object"},
		{"empty body", ``, "JSON object"},
		{"array", `["https://example.com"]`, "JSON object"},
		{"trailing data", `{"url":"https://a.com"}{"url":"https://b.com"}`, "single JSON object"},
		{"too large", `{"url":"https://example.com/` + strings.Repeat("a", maxBodyBytes) + `"}`, "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler()
			rec := doShorten(t, h, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			var e errorResponse
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if !strings.Contains(e.Error, tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", e.Error, tt.wantMsg)
			}
			if h.store.Len() != 0 {
				t.Error("rejected request must not store a link")
			}
		})
	}
}

func TestShorten_StoreFailure(t *testing.T) {
	h := newTestHandler(memory.WithCodeFunc(func(int) (string, error) {
		return "", errors.New("boom")
	}))
	rec := doShorten(t, h, `{"url":"https://example.com/"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestRedirect_NotFound(t *testing.T) {
	h := newTestHandler()
	known := decodeShorten(t, doShorten(t, h, `{"url":"https://example.com/"}`)).Code
	flipped := flipCase(known)
	if flipped == known {
		flipped = "QQQQQQQ"
	}

	tests := []struct{ name, path string }{
		{"unknown valid code", "/zzzzzz"},
		{"unknown 8-char code", "/ZZZZZZZZ"},
		{"known code wrong case", "/" + flipped},
		{"too short", "/abc"},
		{"too long", "/abcdefghij"},
		{"invalid chars", "/abc-12"},
		{"favicon", "/favicon.ico"},
		{"root", "/"},
		{"nested path", "/abcdef/extra"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("GET %s: status = %d, want 404", tt.path, rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Errorf("GET %s: unexpected Location %q", tt.path, loc)
			}
		})
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler()
	tests := []struct{ method, path string }{
		{http.MethodGet, "/api/shorten"},
		{http.MethodPut, "/api/shorten"},
		{http.MethodPost, "/abcdef"},
		{http.MethodDelete, "/abcdef"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", tt.method, tt.path, rec.Code)
		}
	}
}

func TestConcurrentShortenSameURLAndRedirect(t *testing.T) {
	h := newTestHandler()
	const n = 100
	codes := make([]string, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := doShorten(t, h, `{"url":"https://example.com/concurrent"}`)
			if rec.Code != http.StatusCreated {
				t.Errorf("status %d", rec.Code)
				return
			}
			var resp shortenResponse
			json.NewDecoder(rec.Body).Decode(&resp)
			codes[i] = resp.Code

			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/"+resp.Code, nil))
			if r.Code != http.StatusFound {
				t.Errorf("redirect status %d", r.Code)
			}
		}()
	}
	close(start)
	wg.Wait()

	for i, c := range codes {
		if c == "" || c != codes[0] {
			t.Fatalf("request %d got code %q, request 0 got %q", i, c, codes[0])
		}
	}
	if h.store.Len() != 1 {
		t.Errorf("store has %d links, want 1", h.store.Len())
	}
}

func flipCase(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - 32
		case c >= 'A' && c <= 'Z':
			b[i] = c + 32
		}
	}
	return string(b)
}
