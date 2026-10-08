package shortener

import "time"

type Link struct {
	Code      string
	URL       string
	CreatedAt time.Time
}
