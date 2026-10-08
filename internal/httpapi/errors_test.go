package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"url-shortener/internal/shortener"
)

func TestWriteDomainError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"invalid url", fmt.Errorf("%w: url scheme must be http or https", shortener.ErrInvalidURL),
			http.StatusBadRequest, "invalid url: url scheme must be http or https"},
		{"not found", shortener.ErrNotFound, http.StatusNotFound, "not found"},
		{"wrapped not found", fmt.Errorf("db: %w", fmt.Errorf("get %q: %w", "abcdef", shortener.ErrNotFound)),
			http.StatusNotFound, "not found"},
		{"unknown error", errors.New("connection refused to 10.0.0.5:5432"),
			http.StatusInternalServerError, "internal error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeDomainError(rec, tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var e errorResponse
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil {
				t.Fatal(err)
			}
			if e.Error != tt.wantBody {
				t.Errorf("error = %q, want %q", e.Error, tt.wantBody)
			}
		})
	}
}

func TestHandlers_MapStoreErrors(t *testing.T) {
	storeErrs := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", fmt.Errorf("get: %w", shortener.ErrNotFound), http.StatusNotFound},
		{"internal", errors.New("disk on fire"), http.StatusInternalServerError},
	}
	for _, se := range storeErrs {
		t.Run("shorten/"+se.name, func(t *testing.T) {
			fs := &fakeStore{ShortenFn: func(string) (shortener.Link, error) { return shortener.Link{}, se.err }}
			rec := doShorten(t, New(fs, testBase), `{"url":"https://example.com/"}`)
			if rec.Code != se.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, se.wantStatus)
			}
			if strings.Contains(rec.Body.String(), "disk on fire") {
				t.Error("internal error details leaked to client")
			}
		})
		for _, path := range []string{"/abcdef", "/api/v1/links/abcdef"} {
			t.Run("get "+path+"/"+se.name, func(t *testing.T) {
				fs := &fakeStore{GetFn: func(string) (shortener.Link, error) { return shortener.Link{}, se.err }}
				rec := doGet(New(fs, testBase), path)
				if rec.Code != se.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, se.wantStatus)
				}
				if strings.Contains(rec.Body.String(), "disk on fire") {
					t.Error("internal error details leaked to client")
				}
			})
		}
	}
}

func TestShorten_PassesNormalizedURLToStore(t *testing.T) {
	fs := &fakeStore{}
	h := New(fs, testBase)
	resp := decodeShorten(t, doShorten(t, h, `{"url":"  HTTPS://Example.COM:443  "}`))

	shortenCalls, _ := fs.calls()
	if len(shortenCalls) != 1 || shortenCalls[0] != "https://example.com/" {
		t.Errorf("store.Shorten called with %q, want [https://example.com/]", shortenCalls)
	}
	if resp.Code != "fake01" || resp.ShortURL != testBase+"/fake01" {
		t.Errorf("response = %+v", resp)
	}
}

func TestShorten_InvalidURLNeverReachesStore(t *testing.T) {
	fs := &fakeStore{}
	h := New(fs, testBase)
	for _, body := range []string{`{}`, `{"url":"ftp://x.com"}`, `{"url":"/rel"}`, `not json`} {
		if rec := doShorten(t, h, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, rec.Code)
		}
	}
	if shortenCalls, _ := fs.calls(); len(shortenCalls) != 0 {
		t.Errorf("store.Shorten called %d times for invalid input", len(shortenCalls))
	}
}

func TestLookup_InvalidCodeNeverReachesStore(t *testing.T) {
	fs := &fakeStore{}
	h := New(fs, testBase)
	for _, path := range []string{"/favicon.ico", "/abc", "/api/v1/links/abc-12"} {
		if rec := doGet(h, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
	if _, getCalls := fs.calls(); len(getCalls) != 0 {
		t.Errorf("store.Get called with %q for invalid codes", getCalls)
	}
}

func TestRedirectAndMetadata_WithFake(t *testing.T) {
	created := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	fs := &fakeStore{GetFn: func(code string) (shortener.Link, error) {
		if code != "abc123" {
			return shortener.Link{}, shortener.ErrNotFound
		}
		return shortener.Link{Code: code, URL: "https://go.dev/doc/", CreatedAt: created}, nil
	}}
	h := New(fs, testBase)

	rec := doGet(h, "/abc123")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://go.dev/doc/" {
		t.Errorf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = doGet(h, "/api/v1/links/abc123")
	if got, want := strings.TrimSpace(rec.Body.String()),
		`{"url":"https://go.dev/doc/","created_at":"2026-01-15T12:00:00Z"}`; got != want {
		t.Errorf("metadata body = %s, want %s", got, want)
	}

	if rec := doGet(h, "/api/v1/links/zzzzzz"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown code: status %d, want 404", rec.Code)
	}
}
