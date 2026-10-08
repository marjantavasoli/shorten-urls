package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestParseConfig_MaxLinks(t *testing.T) {
	cfg, err := parseConfig(nil, io.Discard)
	if err != nil || cfg.maxLinks != defaultMaxLinks {
		t.Fatalf("default maxLinks = %d, %v; want %d", cfg.maxLinks, err, defaultMaxLinks)
	}
	for _, tt := range []struct {
		arg  string
		want int
	}{{"0", 0}, {"5", 5}} {
		cfg, err := parseConfig([]string{"-max-links", tt.arg}, io.Discard)
		if err != nil || cfg.maxLinks != tt.want {
			t.Errorf("-max-links %s: got %d, %v", tt.arg, cfg.maxLinks, err)
		}
	}
	if _, err := parseConfig([]string{"-max-links", "-1"}, io.Discard); err == nil {
		t.Error("-max-links -1: want error")
	}
}

func TestNewServer_Timeouts(t *testing.T) {
	cfg := defaultConfig()
	srv := newServer(cfg)
	checks := []struct {
		name      string
		got, want time.Duration
	}{
		{"ReadHeaderTimeout", srv.ReadHeaderTimeout, 5 * time.Second},
		{"ReadTimeout", srv.ReadTimeout, 10 * time.Second},
		{"WriteTimeout", srv.WriteTimeout, 10 * time.Second},
		{"IdleTimeout", srv.IdleTimeout, 60 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if srv.MaxHeaderBytes != 16<<10 {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 16<<10)
	}
	if srv.Addr != cfg.addr || srv.Handler == nil {
		t.Errorf("Addr = %q, Handler = %v", srv.Addr, srv.Handler)
	}
}

func startServer(t *testing.T, cfg config) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(cfg)
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func shortTimeoutConfig() config {
	cfg := defaultConfig()
	cfg.readHeaderTimeout = 150 * time.Millisecond
	cfg.readTimeout = 300 * time.Millisecond
	cfg.writeTimeout = 2 * time.Second
	cfg.idleTimeout = 300 * time.Millisecond
	return cfg
}

func TestServer_SlowHeadersAreCutOff(t *testing.T) {
	addr := startServer(t, shortTimeoutConfig())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "GET /abcdef HTTP/1.1\r\nHost: x\r\n"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = io.ReadAll(conn)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("server kept the half-sent request open for 3s; ReadHeaderTimeout not applied")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("connection closed after %v, want about 150ms", elapsed)
	}
}

func TestServer_SlowBodyGets408(t *testing.T) {
	addr := startServer(t, shortTimeoutConfig())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req := "POST /api/shorten HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 40\r\n\r\n{\"url\":"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Errorf("status = %d, want 408", resp.StatusCode)
	}
}

func TestServer_EndToEnd(t *testing.T) {
	cfg := shortTimeoutConfig()
	cfg.maxLinks = 1
	addr := startServer(t, cfg)
	base := "http://" + addr

	post := func(body string) *http.Response {
		t.Helper()
		resp, err := http.Post(base+"/api/shorten", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	resp := post(`{"url":"https://go.dev/doc/"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first shorten: %d", resp.StatusCode)
	}
	var created struct{ Code string }
	json.NewDecoder(resp.Body).Decode(&created)

	if resp := post(`{"url":"https://go.dev/doc/"}`); resp.StatusCode != http.StatusCreated {
		t.Errorf("existing URL when full: %d, want 201", resp.StatusCode)
	}
	if resp := post(`{"url":"https://example.com/"}`); resp.StatusCode != http.StatusInsufficientStorage {
		t.Errorf("new URL when full: %d, want 507", resp.StatusCode)
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := client.Get(base + "/" + created.Code)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "https://go.dev/doc/" {
		t.Errorf("redirect: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
}
