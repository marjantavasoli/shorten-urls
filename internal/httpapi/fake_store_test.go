package httpapi

import (
	"sync"

	"url-shortener/internal/shortener"
)

type fakeStore struct {
	ShortenFn func(longURL string) (shortener.Link, error)
	GetFn     func(code string) (shortener.Link, error)

	mu          sync.Mutex
	shortenArgs []string
	getArgs     []string
}

func (f *fakeStore) Shorten(longURL string) (shortener.Link, error) {
	f.mu.Lock()
	f.shortenArgs = append(f.shortenArgs, longURL)
	f.mu.Unlock()
	if f.ShortenFn == nil {
		return shortener.Link{Code: "fake01", URL: longURL}, nil
	}
	return f.ShortenFn(longURL)
}

func (f *fakeStore) Get(code string) (shortener.Link, error) {
	f.mu.Lock()
	f.getArgs = append(f.getArgs, code)
	f.mu.Unlock()
	if f.GetFn == nil {
		return shortener.Link{}, shortener.ErrNotFound
	}
	return f.GetFn(code)
}

func (f *fakeStore) calls() (shorten, get []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.shortenArgs...), append([]string(nil), f.getArgs...)
}
