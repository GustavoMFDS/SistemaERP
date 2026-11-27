package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func readRateLimitFailures(t *testing.T, policy, reason string) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := rateLimitBackendFailures.WithLabelValues(policy, reason).Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}
func TestRateLimitDiagnosticLabels(t *testing.T) {
	cases := []struct {
		name           string
		err            error
		policy, reason string
	}{
		{"auth_login", context.DeadlineExceeded, "auth_login", "deadline"},
		{"auth_login_identifier", fmt.Errorf("synthetic-secret: %w", context.Canceled), "auth_login_identifier", "canceled"},
		{"auth_login_ip_identifier", ErrRateLimitUnavailable, "auth_login_ip_identifier", "unavailable"},
		{"synthetic-secret", errors.New("synthetic-secret"), "other", "backend-error"},
	}
	for _, c := range cases {
		p, r := rateLimitFailureLabels(c.name, c.err)
		if p != c.policy || r != c.reason {
			t.Fatalf("unexpected labels %q %q", p, r)
		}
	}
}
func TestRateLimitDiagnosticCountsFailClosedOnly(t *testing.T) {
	policy := "auth_login"
	before := readRateLimitFailures(t, policy, "unavailable")
	called := false
	h := RateLimit(nil, policy, 10, time.Minute, true, RateLimitByIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/login", nil))
	if called || rec.Code != 503 || readRateLimitFailures(t, policy, "unavailable") != before+1 {
		t.Fatal("middleware fail-closed metric incorrect")
	}
	before = readRateLimitFailures(t, "auth_login_identifier", "unavailable")
	ok, err := AllowRateLimit(context.Background(), nil, "auth_login_identifier", "synthetic-secret", 10, time.Minute, true)
	if ok || !errors.Is(err, ErrRateLimitUnavailable) || readRateLimitFailures(t, "auth_login_identifier", "unavailable") != before+1 {
		t.Fatal("identifier failure metric incorrect")
	}
	ok, err = AllowRateLimit(context.Background(), nil, "auth_login_identifier", "synthetic-secret", 10, time.Minute, false)
	if !ok || err != nil || readRateLimitFailures(t, "auth_login_identifier", "unavailable") != before+1 {
		t.Fatal("development fallback wrongly counted")
	}
}
