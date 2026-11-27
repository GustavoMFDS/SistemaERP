package middleware

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

var rateLimitBackendFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "sistemaemgo",
	Subsystem: "security",
	Name:      "rate_limit_backend_failures_total",
	Help:      "Fail-closed rate limiter backend failures by bounded policy and reason.",
}, []string{"policy", "reason"})

// Only fixed internal policies and typed reasons may enter telemetry.
func rateLimitFailureLabels(name string, err error) (string, string) {
	switch name {
	case "auth_login", "auth_login_identifier", "auth_login_ip_identifier", "auth_refresh", "auth_logout", "sales_create", "product_import", "fiscal", "startup_contract":
	default:
		name = "other"
	}
	reason := "backend-error"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		reason = "deadline"
	case errors.Is(err, context.Canceled):
		reason = "canceled"
	case errors.Is(err, ErrRateLimitUnavailable):
		reason = "unavailable"
	}
	return name, reason
}
func recordRateLimitFailure(name string, err error) {
	metricsOnce.Do(initMetrics)
	policy, reason := rateLimitFailureLabels(name, err)
	rateLimitBackendFailures.WithLabelValues(policy, reason).Inc()
}
