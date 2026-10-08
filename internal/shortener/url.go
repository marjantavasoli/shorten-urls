package shortener

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const MaxURLLength = 2048

func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("%w: url is required", ErrInvalidURL)
	}
	if len(s) > MaxURLLength {
		return "", fmt.Errorf("%w: url is longer than %d bytes", ErrInvalidURL, MaxURLLength)
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: url is malformed", ErrInvalidURL)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: url scheme must be http or https", ErrInvalidURL)
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return "", fmt.Errorf("%w: url must be absolute and include a host", ErrInvalidURL)
	}

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	u.Scheme = scheme
	u.Host = host
	if u.Path == "" && u.RawPath == "" {
		u.Path = "/"
	}
	return u.String(), nil
}
