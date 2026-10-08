package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"url-shortener/internal/store/memory"
)

func benchHandlerWithLink(b *testing.B) (*Handler, string) {
	b.Helper()
	h := New(memory.New(), testBase)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/shorten",
		strings.NewReader(`{"url":"https://go.dev/doc/"}`)))
	if rec.Code != http.StatusCreated {
		b.Fatalf("setup: status %d", rec.Code)
	}
	body := rec.Body.String()
	i := strings.Index(body, `"code":"`) + len(`"code":"`)
	return h, body[i : i+strings.IndexByte(body[i:], '"')]
}

func BenchmarkHTTP_Shorten_New(b *testing.B) {
	h := New(memory.New(), testBase)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := fmt.Sprintf(`{"url":"https://example.com/articles/%d"}`, i)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(body)))
		if rec.Code != http.StatusCreated {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func BenchmarkHTTP_Shorten_Existing(b *testing.B) {
	h, _ := benchHandlerWithLink(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/shorten",
			strings.NewReader(`{"url":"https://go.dev/doc/"}`)))
		if rec.Code != http.StatusCreated {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func BenchmarkHTTP_Redirect(b *testing.B) {
	h, code := benchHandlerWithLink(b)
	req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			b.Fatalf("status %d", rec.Code)
		}
	}
}

func BenchmarkHTTP_Metadata(b *testing.B) {
	h, code := benchHandlerWithLink(b)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/"+code, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
