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

	"url-shortener/internal/httpapi"
	"url-shortener/internal/store/memory"
)

type config struct {
	addr string
	base string
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
	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: httpapi.New(memory.New(), cfg.base),
	}
	log.Printf("listening on %s (base %s)", cfg.addr, cfg.base)
	return srv.ListenAndServe()
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.addr, "addr", ":8080", "listen address")
	fs.StringVar(&cfg.base, "base", "http://localhost:8080", "public base URL used to build short_url")
	if err := fs.Parse(args); err != nil {
		return config{}, err
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
