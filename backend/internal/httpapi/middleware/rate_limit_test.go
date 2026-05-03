package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimitReturnsStandardJSON429(t *testing.T) {
	handler := RateLimit(nil, "test", 1, time.Minute, RateLimitByIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	first := httptest.NewRequest(http.MethodPost, "/login", nil)
	first.RemoteAddr = "192.0.2.10:1234"
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want 204", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPost, "/login", nil)
	second.RemoteAddr = "192.0.2.10:1235"
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", secondRec.Code)
	}
	if secondRec.Body.String() == "" || secondRec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected standardized JSON rate-limit response")
	}
}

func TestHashRateLimitIdentifierNormalizesAndHidesEmail(t *testing.T) {
	first := HashRateLimitIdentifier("  John@Example.COM ")
	second := HashRateLimitIdentifier("john@example.com")
	if first != second {
		t.Fatalf("expected normalized identifiers to hash equally")
	}
	if strings.Contains(first, "john") || strings.Contains(first, "@") || len(first) != 64 {
		t.Fatalf("hash leaks raw identifier or has unexpected shape: %q", first)
	}
}
