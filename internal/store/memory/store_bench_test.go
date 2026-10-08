package memory

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"url-shortener/internal/shortener"
)

const benchLinks = 100_000

func benchURL(i int) string {
	return fmt.Sprintf("https://example.com/articles/2026/10/some-typical-slug-%d?utm_source=bench", i)
}

func filledStore(b *testing.B, n int) (*Store, []string) {
	b.Helper()
	s := New()
	codes := make([]string, n)
	for i := range n {
		l, err := s.Shorten(benchURL(i))
		if err != nil {
			b.Fatal(err)
		}
		codes[i] = l.Code
	}
	return s, codes
}

func BenchmarkShorten_New(b *testing.B) {
	s := New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Shorten(benchURL(i)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShorten_Existing(b *testing.B) {
	s, _ := filledStore(b, 1000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Shorten(benchURL(i % 1000)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	s, codes := filledStore(b, benchLinks)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Get(codes[i%len(codes)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGet_Parallel(b *testing.B) {
	s, codes := filledStore(b, benchLinks)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, err := s.Get(codes[i%len(codes)]); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func BenchmarkMixed_Parallel(b *testing.B) {
	s, codes := filledStore(b, benchLinks)
	var next atomic.Int64
	next.Store(benchLinks)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%100 == 0 {
				if _, err := s.Shorten(benchURL(int(next.Add(1)))); err != nil {
					b.Fatal(err)
				}
			} else if _, err := s.Get(codes[i%len(codes)]); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

type rwLocker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
}

type plainMutex struct{ sync.Mutex }

func (m *plainMutex) RLock()   { m.Lock() }
func (m *plainMutex) RUnlock() { m.Unlock() }

func BenchmarkLock_ReadHeavy(b *testing.B) {
	locks := []struct {
		name string
		mk   func() rwLocker
	}{
		{"Mutex", func() rwLocker { return &plainMutex{} }},
		{"RWMutex", func() rwLocker { return &sync.RWMutex{} }},
	}
	for _, lk := range locks {
		b.Run(lk.name, func(b *testing.B) {
			mu := lk.mk()
			m := make(map[string]shortener.Link, benchLinks)
			keys := make([]string, benchLinks)
			for i := range keys {
				keys[i] = fmt.Sprintf("k%07d", i)
				m[keys[i]] = shortener.Link{Code: keys[i], URL: benchURL(i)}
			}
			var next atomic.Int64
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					if i%100 == 0 {
						k := fmt.Sprintf("w%d", next.Add(1))
						mu.Lock()
						m[k] = shortener.Link{Code: k}
						mu.Unlock()
					} else {
						mu.RLock()
						_ = m[keys[i%len(keys)]]
						mu.RUnlock()
					}
					i++
				}
			})
		})
	}
}

func BenchmarkMemoryPerLink(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		s, _ := filledStore(b, benchLinks)
		runtime.GC()
		runtime.ReadMemStats(&after)
		b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/benchLinks, "B/link")
		runtime.KeepAlive(s)
	}
}
