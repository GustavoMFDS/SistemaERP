package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/redis/go-redis/v9"
)

type securityDiagnosticHook struct {
	mode    string
	consume atomic.Int64
	rate    atomic.Int64
}

var diagnosticSyntheticSecret = errors.New("synthetic-secret-not-for-logs")

func (h *securityDiagnosticHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected network access")
	}
}
func (h *securityDiagnosticHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(context.Context, []redis.Cmder) error { return errors.New("unexpected pipeline") }
}
func (h *securityDiagnosticHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		switch cmd.Name() {
		case "set":
			if h.mode == "set-error" {
				return diagnosticSyntheticSecret
			}
			cmd.(*redis.StatusCmd).SetVal("OK")
		case "del":
			cmd.(*redis.IntCmd).SetVal(1)
		case "eval":
			args := cmd.Args()
			script := fmt.Sprint(args[1])
			var result int64
			if strings.Contains(script, "INCR") {
				if h.mode == "rate-budget" {
					<-ctx.Done()
					return ctx.Err()
				}
				if h.mode == "rate-deadline" {
					return fmt.Errorf("synthetic-secret-not-for-logs: %w", context.DeadlineExceeded)
				}
				if h.mode == "rate-canceled" {
					return context.Canceled
				}
				if h.mode == "rate-error" {
					return diagnosticSyntheticSecret
				}
				result = h.rate.Add(1)
				if h.mode == "rate-count" {
					result = 1
				}
			} else if fmt.Sprint(args[len(args)-1]) == "invalid-user" {
				if h.mode == "isolation-error" {
					return diagnosticSyntheticSecret
				}
				if h.mode == "isolation-accepted" {
					result = 1
				}
			} else {
				if h.mode == "consume-deadline" {
					return context.DeadlineExceeded
				}
				if h.mode == "consume-error" {
					return diagnosticSyntheticSecret
				}
				if h.consume.Add(1) == 1 || h.mode == "consume-count" {
					result = 1
				}
			}
			cmd.(*redis.Cmd).SetVal(result)
		default:
			return errors.New("unexpected command")
		}
		return nil
	}
}

func TestRedisSecurityDiagnosticProbe(t *testing.T) {
	cases := []struct{ mode, stage, reason string }{
		{"normal", "", ""},
		{"set-error", "refresh-persistence", "backend-error"},
		{"isolation-error", "refresh-user-isolation", "backend-error"},
		{"isolation-accepted", "refresh-user-isolation", "invariant"},
		{"consume-error", "refresh-atomic-consume", "backend-error"},
		{"consume-count", "refresh-atomic-consume", "invariant"},
		{"rate-error", "rate-limit-atomic", "backend-error"},
		{"rate-deadline", "rate-limit-atomic", "deadline"},
		{"rate-canceled", "rate-limit-atomic", "canceled"},
		{"consume-deadline", "refresh-atomic-consume", "deadline"},
		{"rate-count", "rate-limit-atomic", "invariant"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
			defer c.Close()
			c.AddHook(&securityDiagnosticHook{mode: tc.mode})
			err := verifyRedisSecurityPrimitives(context.Background(), c)
			if (err == nil) != (tc.stage == "") {
				t.Fatalf("unexpected probe outcome: %v", err)
			}
			if err != nil {
				if strings.Contains(err.Error(), diagnosticSyntheticSecret.Error()) {
					t.Fatal("raw cause disclosed")
				}
				err = startupFailure(StartupRedisSecurity, fmt.Errorf("wrapped: %w", err))
			}
			stage, reason := RedisSecurityDiagnostic(err)
			if stage != tc.stage || reason != tc.reason {
				t.Fatalf("diagnostic %q/%q want %q/%q", stage, reason, tc.stage, tc.reason)
			}
		})
	}
}
func TestRedisSecurityDiagnosticAllowlist(t *testing.T) {
	err := verifyRedisSecurityPrimitives(context.Background(), nil)
	if stage, reason := RedisSecurityDiagnostic(err); stage != "backend" || reason != "missing" {
		t.Fatal("missing backend misclassified")
	}
	for _, err := range []error{nil, diagnosticSyntheticSecret, &redisSecurityProbeError{stage: "unapproved", reason: "invariant"}, &redisSecurityProbeError{stage: "backend", reason: "unapproved"}} {
		if stage, reason := RedisSecurityDiagnostic(err); stage != "" || reason != "" {
			t.Fatal("unapproved diagnostic escaped")
		}
	}
	for _, tc := range []struct {
		cause error
		want  string
	}{{context.DeadlineExceeded, "deadline"}, {context.Canceled, "canceled"}} {
		err := securityProbeFailure("refresh-persistence", tc.cause, "")
		if _, reason := RedisSecurityDiagnostic(err); reason != tc.want || !errors.Is(err, tc.cause) {
			t.Fatal("typed cause lost")
		}
	}
	var counts securityProbeErrorCounts
	counts.record(context.Canceled)
	if _, reason := RedisSecurityDiagnostic(counts.failure("rate-limit-atomic")); reason != "canceled" {
		t.Fatal("concurrent cancellation not classified")
	}
	counts.record(diagnosticSyntheticSecret)
	if _, reason := RedisSecurityDiagnostic(counts.failure("rate-limit-atomic")); reason != "backend-error" {
		t.Fatal("mixed errors misclassified")
	}
	counts.reset()
	if _, reason := RedisSecurityDiagnostic(counts.failure("rate-limit-atomic")); reason != "invariant" {
		t.Fatal("counts not reset")
	}
}

func TestRedisSecurityDiagnosticRateBudget(t *testing.T) {
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer c.Close()
	c.AddHook(&securityDiagnosticHook{mode: "rate-budget"})
	err := verifyRedisSecurityPrimitives(context.Background(), c)
	stage, reason := RedisSecurityDiagnostic(err)
	if stage != "rate-limit-atomic" || reason != "deadline" {
		t.Fatalf("diagnostic %q/%q", stage, reason)
	}
}
