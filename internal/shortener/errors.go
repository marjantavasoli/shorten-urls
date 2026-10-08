package shortener

import "errors"

var (
	ErrInvalidURL = errors.New("invalid url")
	ErrNotFound   = errors.New("link not found")
)
