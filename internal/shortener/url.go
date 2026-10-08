package shortener

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const MaxURLLength = 2048

func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("url is required")
	}
	if len(s) > MaxURLLength {
		return "", fmt.Errorf("url is longer than %d bytes", MaxURLLength)
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", errors.New("url is malformed")
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("url scheme must be http or https")
	}
	if u.Opaque != "" || u.Hostname() == "" {
		return "", errors.New("url must be absolute and include a host")
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
