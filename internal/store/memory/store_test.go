package memory

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"url-shortener/internal/shortener"
)

func TestShorten_NewURL(t *testing.T) {
	fixed := time.Date(2026, 1, 15, 12, 0, 0, 0, time.FixedZone("x", 3600))
	s := New(WithClock(func() time.Time { return fixed }))

	l, err := s.Shorten("https://go.dev/doc/")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Code) != 6 || !shortener.IsValidCode(l.Code) {
		t.Errorf("code %q: want 6 base62 chars", l.Code)
	}
	if l.URL != "https://go.dev/doc/" {
		t.Errorf("URL = %q", l.URL)
	}
	if !l.CreatedAt.Equal(fixed) || l.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want %v in UTC", l.CreatedAt, fixed)
	}

	got, err := s.Get(l.Code)
	if err != nil || got != l {
		t.Errorf("Get(%q) = %+v, %v; want %+v, nil", l.Code, got, err, l)
	}
}

func TestShorten_SameURLSameCode(t *testing.T) {
	s := New()
	a, _ := s.Shorten("https://example.com/")
	b, _ := s.Shorten("https://example.com/")
	if a != b {
		t.Errorf("same URL gave different links: %+v vs %+v", a, b)
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
}

func TestShorten_DistinctURLsDistinctCodes(t *testing.T) {
	s := New()
	a, _ := s.Shorten("https://example.com/a")
	b, _ := s.Shorten("https://example.com/b")
	if a.Code == b.Code {
		t.Errorf("distinct URLs share code %q", a.Code)
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := New().Get("zzzzzz")
	if !errors.Is(err, shortener.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "zzzzzz") {
		t.Errorf("err = %q, want it to mention the code", err)
	}
}

func TestShorten_CollisionRetry(t *testing.T) {
	seq := []string{"AAAAAA", "AAAAAA", "AAAAAA", "BBBBBB"}
	i := 0
	s := New(WithCodeFunc(func(int) (string, error) {
		c := seq[i]
		i++
		return c, nil
	}))

	a, _ := s.Shorten("https://example.com/a")
	b, err := s.Shorten("https://example.com/b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != "AAAAAA" || b.Code != "BBBBBB" {
		t.Errorf("codes = %q, %q; want AAAAAA, BBBBBB", a.Code, b.Code)
	}
}

func TestShorten_LengthEscalation(t *testing.T) {
	var lengths []int
	s := New(WithCodeFunc(func(n int) (string, error) {
		lengths = append(lengths, n)
		return strings.Repeat("A", n), nil
	}))

	for i, want := range []int{6, 7, 8} {
		l, err := s.Shorten(fmt.Sprintf("https://example.com/%d", i))
		if err != nil {
			t.Fatalf("URL %d: %v", i, err)
		}
		if len(l.Code) != want {
			t.Errorf("URL %d: code %q has length %d, want %d", i, l.Code, len(l.Code), want)
		}
	}

	lengths = nil
	if _, err := s.Shorten("https://example.com/full"); err == nil {
		t.Fatal("want error when no code is free")
	}
	want := []int{6, 6, 6, 7, 7, 7, 8, 8, 8}
	if fmt.Sprint(lengths) != fmt.Sprint(want) {
		t.Errorf("attempted lengths %v, want %v", lengths, want)
	}
	if s.Len() != 3 {
		t.Errorf("Len = %d, want 3 (failed shorten must not store anything)", s.Len())
	}
}

func TestShorten_ExistingURLSkipsGeneration(t *testing.T) {
	calls := 0
	s := New(WithCodeFunc(func(n int) (string, error) {
		calls++
		return shortener.RandomCode(n)
	}))
	s.Shorten("https://example.com/")
	s.Shorten("https://example.com/")
	s.Shorten("https://example.com/")
	if calls != 1 {
		t.Errorf("code generator called %d times, want 1 (only for the new URL)", calls)
	}
}

func TestShorten_GeneratorError(t *testing.T) {
	boom := errors.New("entropy exhausted")
	s := New(WithCodeFunc(func(int) (string, error) { return "", boom }))
	_, err := s.Shorten("https://example.com/")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}

func TestShorten_ConcurrentSameURL(t *testing.T) {
	s := New()
	const n = 200
	codes := make([]string, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			l, err := s.Shorten("https://example.com/same")
			if err != nil {
				t.Error(err)
				return
			}
			codes[i] = l.Code
		}()
	}
	close(start)
	wg.Wait()

	for i, c := range codes {
		if c != codes[0] {
			t.Fatalf("goroutine %d got %q, goroutine 0 got %q", i, c, codes[0])
		}
	}
	if s.Len() != 1 {
		t.Errorf("Len = %d, want 1", s.Len())
	}
}

func TestShorten_ConcurrentDistinctURLsAndReads(t *testing.T) {
	s := New()
	const n = 200
	var mu sync.Mutex
	seen := map[string]string{}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(2)
		u := fmt.Sprintf("https://example.com/%d", i)
		go func() {
			defer wg.Done()
			l, err := s.Shorten(u)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			if prev, dup := seen[l.Code]; dup {
				t.Errorf("code %q assigned to %q and %q", l.Code, prev, u)
			}
			seen[l.Code] = u
			mu.Unlock()
			if got, err := s.Get(l.Code); err != nil || got.URL != u {
				t.Errorf("Get(%q) = %+v, %v", l.Code, got, err)
			}
		}()
		go func() {
			defer wg.Done()
			s.Get("abcdef")
			s.Len()
		}()
	}
	wg.Wait()
	if s.Len() != n {
		t.Errorf("Len = %d, want %d", s.Len(), n)
	}
}
