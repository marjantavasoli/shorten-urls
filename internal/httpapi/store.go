package httpapi

import "url-shortener/internal/shortener"

type Store interface {
	Shorten(longURL string) (shortener.Link, error)
	Get(code string) (shortener.Link, error)
}
