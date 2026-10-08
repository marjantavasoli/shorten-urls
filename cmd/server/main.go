package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"url-shortener/internal/httpapi"
	"url-shortener/internal/store/memory"
)

const defaultMaxLinks = 1_000_000

type config struct {
	addr     string
	base     string
	maxLinks int

	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	maxHeaderBytes    int
}

func defaultConfig() config {
	return config{
		addr:              ":8080",
		base:              "http://localhost:8080",
		maxLinks:          defaultMaxLinks,
		readHeaderTimeout: 5 * time.Second,
		readTimeout:       10 * time.Second,
		writeTimeout:      10 * time.Second,
		idleTimeout:       60 * time.Second,
		maxHeaderBytes:    16 << 10,
	}
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	srv := newServer(cfg)
	log.Printf("listening on %s (base %s, max links %d)", cfg.addr, cfg.base, cfg.maxLinks)
	return srv.ListenAndServe()
}

func newServer(cfg config) *http.Server {
	store := memory.New(memory.WithMaxLinks(cfg.maxLinks))
	return &http.Server{
		Addr:              cfg.addr,
		Handler:           httpapi.New(store, cfg.base),
		ReadHeaderTimeout: cfg.readHeaderTimeout,
		ReadTimeout:       cfg.readTimeout,
		WriteTimeout:      cfg.writeTimeout,
		IdleTimeout:       cfg.idleTimeout,
		MaxHeaderBytes:    cfg.maxHeaderBytes,
	}
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := defaultConfig()
	fs.StringVar(&cfg.addr, "addr", cfg.addr, "listen address")
	fs.StringVar(&cfg.base, "base", cfg.base, "public base URL used to build short_url")
	fs.IntVar(&cfg.maxLinks, "max-links", cfg.maxLinks, "maximum links kept in memory; new URLs get 507 when full (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.maxLinks < 0 {
		return config{}, fmt.Errorf("invalid -max-links %d: must be >= 0", cfg.maxLinks)
	}
	base, err := validateBase(cfg.base)
	if err != nil {
		return config{}, fmt.Errorf("invalid -base %q: %w", cfg.base, err)
	}
	cfg.base = base
	return cfg, nil
}

func validateBase(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("scheme must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("host is required")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("query and fragment are not allowed")
	}
	return strings.TrimRight(raw, "/"), nil
}
