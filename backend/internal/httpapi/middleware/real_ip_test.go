package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/sistemaemgo/internal/config"
)

func TestTrustedRealIPIgnoresSpoofedHeadersFromUntrustedClient(t *testing.T) {
	cfg := config.Config{TrustedProxyCIDRs: []string{"10.0.0.0/8"}}
	var got string
	h := TrustedRealIP(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = RateLimitByIP(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.10" {
		t.Fatalf("expected socket IP, got %q", got)
	}
}

func TestTrustedRealIPAcceptsForwardedIPFromTrustedProxy(t *testing.T) {
	cfg := config.Config{TrustedProxyCIDRs: []string{"10.0.0.0/8"}}
	var got string
	h := TrustedRealIP(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = RateLimitByIP(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 10.1.2.3")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.8" {
		t.Fatalf("expected forwarded client IP, got %q", got)
	}
}

func TestTrustedRealIPIgnoresInjectedLeftmostForwardedIP(t *testing.T) {
	cfg := config.Config{TrustedProxyCIDRs: []string{"10.0.0.0/8"}}
	var got string
	h := TrustedRealIP(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = RateLimitByIP(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:4321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.8, 10.2.3.4")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.8" {
		t.Fatalf("expected nearest untrusted client IP, got %q", got)
	}
}
