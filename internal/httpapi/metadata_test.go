package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"url-shortener/internal/store/memory"
)

func doGet(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestMetadata_OK(t *testing.T) {
	tehran := time.FixedZone("IRST", 3*3600+1800)
	created := time.Date(2026, 1, 15, 15, 30, 0, 123456789, tehran)
	h := New(memory.New(memory.WithClock(func() time.Time { return created })), testBase)

	code := decodeShorten(t, doShorten(t, h, `{"url":"HTTPS://Go.DEV/doc/"}`)).Code

	rec := doGet(h, "/api/v1/links/"+code)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"url":        "https://go.dev/doc/",
		"created_at": "2026-01-15T12:00:00Z",
	}
	if len(raw) != len(want) {
		t.Errorf("body has keys %v, want exactly url and created_at", raw)
	}
	for k, v := range want {
		if raw[k] != v {
			t.Errorf("%s = %v, want %v", k, raw[k], v)
		}
	}
	if _, err := time.Parse(time.RFC3339, raw["created_at"].(string)); err != nil {
		t.Errorf("created_at is not RFC3339: %v", err)
	}
}

func TestMetadata_DoesNotRedirect(t *testing.T) {
	h := newTestHandler()
	code := decodeShorten(t, doShorten(t, h, `{"url":"https://example.com/"}`)).Code
	rec := doGet(h, "/api/v1/links/"+code)
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("metadata route set Location %q", loc)
	}
}

func TestMetadata_NotFound(t *testing.T) {
	h := newTestHandler()
	tests := []struct{ name, path string }{
		{"unknown code", "/api/v1/links/zzzzzz"},
		{"too short", "/api/v1/links/abc"},
		{"too long", "/api/v1/links/abcdefghij"},
		{"invalid chars", "/api/v1/links/abc-12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doGet(h, tt.path)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			var e errorResponse
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e.Error != "not found" {
				t.Errorf("body = %+v, %v; want {\"error\":\"not found\"}", e, err)
			}
		})
	}
}
