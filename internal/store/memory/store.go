package memory

import (
	"fmt"
	"sync"
	"time"

	"url-shortener/internal/shortener"
)

const attemptsPerLength = 3

var codeLengths = []int{6, 7, 8}

type CodeFunc func(length int) (string, error)

type Option func(*Store)

func WithCodeFunc(f CodeFunc) Option { return func(s *Store) { s.newCode = f } }

func WithClock(now func() time.Time) Option { return func(s *Store) { s.now = now } }

func WithMaxLinks(n int) Option { return func(s *Store) { s.maxLinks = n } }

type Store struct {
	mu     sync.RWMutex
	byCode map[string]shortener.Link
	byURL  map[string]string

	newCode  CodeFunc
	now      func() time.Time
	maxLinks int
}

func New(opts ...Option) *Store {
	s := &Store{
		byCode:  make(map[string]shortener.Link),
		byURL:   make(map[string]string),
		newCode: shortener.RandomCode,
		now:     time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Store) Shorten(longURL string) (shortener.Link, error) {
	if l, ok := s.lookupURL(longURL); ok {
		return l, nil
	}

	for _, length := range codeLengths {
		for range attemptsPerLength {
			code, err := s.newCode(length)
			if err != nil {
				return shortener.Link{}, fmt.Errorf("generate code: %w", err)
			}

			s.mu.Lock()
			if existing, ok := s.byURL[longURL]; ok {
				l := s.byCode[existing]
				s.mu.Unlock()
				return l, nil
			}
			if s.maxLinks > 0 && len(s.byCode) >= s.maxLinks {
				s.mu.Unlock()
				return shortener.Link{}, fmt.Errorf("%d links stored: %w", s.maxLinks, shortener.ErrStoreFull)
			}
			if _, taken := s.byCode[code]; taken {
				s.mu.Unlock()
				continue
			}
			l := shortener.Link{Code: code, URL: longURL, CreatedAt: s.now().UTC()}
			s.byCode[code] = l
			s.byURL[longURL] = code
			s.mu.Unlock()
			return l, nil
		}
	}
	return shortener.Link{}, fmt.Errorf("no free code after %d attempts at lengths %v",
		attemptsPerLength*len(codeLengths), codeLengths)
}

func (s *Store) Get(code string) (shortener.Link, error) {
	s.mu.RLock()
	l, ok := s.byCode[code]
	s.mu.RUnlock()
	if !ok {
		return shortener.Link{}, fmt.Errorf("get %q: %w", code, shortener.ErrNotFound)
	}
	return l, nil
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byCode)
}

func (s *Store) lookupURL(longURL string) (shortener.Link, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	code, ok := s.byURL[longURL]
	if !ok {
		return shortener.Link{}, false
	}
	return s.byCode[code], true
}
