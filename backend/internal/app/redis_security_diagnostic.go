package app

import (
	"context"
	"errors"
	"sync/atomic"
)

// Only these fixed labels may leave the security probe in startup diagnostics.
type redisSecurityProbeError struct {
	stage  string
	reason string
	cause  error
}

func (e *redisSecurityProbeError) Error() string {
	return "redis security probe failed: " + e.stage + "/" + e.reason
}
func (e *redisSecurityProbeError) Unwrap() error { return e.cause }

func securityProbeFailure(stage string, cause error, fallback string) error {
	reason := fallback
	if cause != nil {
		reason = redisStartupPingFailureKind(cause)
		if reason == "other" {
			reason = "backend-error"
		}
	}
	return &redisSecurityProbeError{stage: stage, reason: reason, cause: cause}
}

type securityProbeErrorCounts struct {
	total    atomic.Int32
	deadline atomic.Int32
	canceled atomic.Int32
}

func (c *securityProbeErrorCounts) record(err error) {
	c.total.Add(1)
	if errors.Is(err, context.DeadlineExceeded) {
		c.deadline.Add(1)
	} else if errors.Is(err, context.Canceled) {
		c.canceled.Add(1)
	}
}
func (c *securityProbeErrorCounts) reset() {
	c.total.Store(0)
	c.deadline.Store(0)
	c.canceled.Store(0)
}
func (c *securityProbeErrorCounts) failure(stage string) error {
	n := c.total.Load()
	if n == 0 {
		return securityProbeFailure(stage, nil, "invariant")
	}
	if c.deadline.Load() == n {
		return securityProbeFailure(stage, context.DeadlineExceeded, "")
	}
	if c.canceled.Load() == n {
		return securityProbeFailure(stage, context.Canceled, "")
	}
	return securityProbeFailure(stage, nil, "backend-error")
}

// RedisSecurityDiagnostic returns only allowlisted diagnostic labels, never raw errors.
func RedisSecurityDiagnostic(err error) (string, string) {
	var e *redisSecurityProbeError
	if !errors.As(err, &e) || e == nil {
		return "", ""
	}
	switch e.stage {
	case "backend", "refresh-persistence", "refresh-user-isolation", "refresh-atomic-consume", "rate-limit-atomic":
	default:
		return "", ""
	}
	switch e.reason {
	case "missing", "invariant", "backend-error", "deadline", "canceled", "certificate-validation", "redis-response", "tls-protocol", "transport-transient":
	default:
		return "", ""
	}
	return e.stage, e.reason
}
