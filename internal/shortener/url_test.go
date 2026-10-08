package shortener

import (
	"strings"
	"testing"
)

func TestNormalizeURL_Valid(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"unchanged", "https://go.dev/doc/", "https://go.dev/doc/"},
		{"trim whitespace", "  https://go.dev/doc/ \n", "https://go.dev/doc/"},
		{"lowercase scheme and host", "HTTPS://Go.DEV/doc", "https://go.dev/doc"},
		{"empty path becomes slash", "https://example.com", "https://example.com/"},
		{"drop default http port", "http://example.com:80/x", "http://example.com/x"},
		{"drop default https port", "https://example.com:443/x", "https://example.com/x"},
		{"keep non-default port", "http://example.com:8080/x", "http://example.com:8080/x"},
		{"keep https on port 80", "https://example.com:80/x", "https://example.com:80/x"},
		{"keep path case", "https://example.com/Doc", "https://example.com/Doc"},
		{"keep trailing slash", "https://example.com/doc/", "https://example.com/doc/"},
		{"keep no trailing slash", "https://example.com/doc", "https://example.com/doc"},
		{"keep query order", "https://example.com/?b=1&a=2", "https://example.com/?b=1&a=2"},
		{"keep fragment", "https://example.com/p#Section", "https://example.com/p#Section"},
		{"keep encoded path", "https://example.com/a%2Fb", "https://example.com/a%2Fb"},
		{"ipv6 with port", "http://[::1]:8080/", "http://[::1]:8080/"},
		{"ipv6 default port dropped", "http://[::1]:80/", "http://[::1]/"},
		{"ipv6 no port", "http://[::1]/", "http://[::1]/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.in)
			if err != nil {
				t.Fatalf("NormalizeURL(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeURL_EquivalentFormsMatch(t *testing.T) {
	forms := []string{
		"https://go.dev/",
		"https://go.dev",
		"HTTPS://GO.DEV/",
		"https://go.dev:443/",
		" https://go.dev/ ",
	}
	want, _ := NormalizeURL(forms[0])
	for _, f := range forms[1:] {
		if got, _ := NormalizeURL(f); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", f, got, want)
		}
	}
}

func TestNormalizeURL_Invalid(t *testing.T) {
	tests := []struct{ name, in string }{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no scheme", "example.com/path"},
		{"relative path", "/foo/bar"},
		{"ftp scheme", "ftp://example.com/file"},
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,<script>alert(1)</script>"},
		{"mailto scheme", "mailto:a@example.com"},
		{"opaque http", "http:example.com"},
		{"missing host", "http:///path"},
		{"port only", "http://:8080/"},
		{"malformed", "http://exa mple.com/"},
		{"bad port", "http://example.com:abc/"},
		{"control char", "http://example.com/\x7f"},
		{"too long", "https://example.com/" + strings.Repeat("a", MaxURLLength)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := NormalizeURL(tt.in); err == nil {
				t.Errorf("NormalizeURL(%q) = %q, want error", tt.in, got)
			}
		})
	}
}
