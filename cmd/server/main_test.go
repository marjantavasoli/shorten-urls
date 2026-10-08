package main

import (
	"io"
	"testing"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parseConfig(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.addr != ":8080" || cfg.base != "http://localhost:8080" {
		t.Errorf("defaults = %+v", cfg)
	}
}

func TestParseConfig_Flags(t *testing.T) {
	tests := []struct {
		args     []string
		wantAddr string
		wantBase string
	}{
		{[]string{"-addr", ":9090", "-base", "https://sho.rt"}, ":9090", "https://sho.rt"},
		{[]string{"-base", "https://sho.rt/"}, ":8080", "https://sho.rt"},
		{[]string{"-base", "https://example.com/s/"}, ":8080", "https://example.com/s"},
	}
	for _, tt := range tests {
		cfg, err := parseConfig(tt.args, io.Discard)
		if err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if cfg.addr != tt.wantAddr || cfg.base != tt.wantBase {
			t.Errorf("%v: got %+v", tt.args, cfg)
		}
	}
}

func TestParseConfig_InvalidBase(t *testing.T) {
	for _, base := range []string{
		"localhost:8080",
		"ftp://example.com",
		"http://",
		"/relative",
		"http://a.com/?x=1",
		"http://a.com/#frag",
		"http://exa mple.com",
	} {
		if _, err := parseConfig([]string{"-base", base}, io.Discard); err == nil {
			t.Errorf("-base %q: want error", base)
		}
	}
}

func TestParseConfig_UnknownFlag(t *testing.T) {
	if _, err := parseConfig([]string{"-nope"}, io.Discard); err == nil {
		t.Error("want error for unknown flag")
	}
}

func TestRun_Errors(t *testing.T) {
	if err := run([]string{"-base", "nope"}, io.Discard); err == nil {
		t.Error("run with bad -base: want error")
	}
	if err := run([]string{"-addr", ":-1"}, io.Discard); err == nil {
		t.Error("run with bad -addr: want listen error")
	}
}
