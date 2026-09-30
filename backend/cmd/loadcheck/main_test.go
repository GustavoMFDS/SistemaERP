package main

import (
	"net/http"
	"testing"
	"time"
)

func TestValidateConfigBlocksWritesByDefault(t *testing.T) {
	cfg := config{
		URL:          "http://example.test",
		Method:       http.MethodPost,
		Duration:     time.Second,
		Concurrency:  1,
		Timeout:      time.Second,
		MaxErrorRate: 0.01,
		MaxP95:       time.Second,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected write method to require explicit opt-in")
	}
	cfg.AllowWrites = true
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("expected opted-in write method to validate: %v", err)
	}
}

func TestPercentile(t *testing.T) {
	values := []time.Duration{10, 20, 30, 40, 50}
	if got := percentile(values, 0.95); got != 40 {
		t.Fatalf("p95=%v, want 40", got)
	}
	if got := percentile(values, 1); got != 50 {
		t.Fatalf("p100=%v, want 50", got)
	}
}

func TestParseHeaders(t *testing.T) {
	h, err := parseHeaders([]string{"Authorization: Bearer token", "X-Test: one"})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("Authorization=%q", got)
	}
	if _, err := parseHeaders([]string{"broken"}); err == nil {
		t.Fatal("expected malformed header to fail")
	}
}
