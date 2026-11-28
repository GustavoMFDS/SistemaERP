package main

import (
	"context"
	"errors"
	"github.com/example/sistemaemgo/internal/app"
	"strings"
	"testing"
)

func TestRedisStartupDiagnosticMessage(t *testing.T) {
	for _, tc := range []struct {
		stage                  app.StartupStage
		substage, reason, want string
	}{
		{app.StartupRedisSecurity, "rate-limit-atomic", "deadline", "substage=rate-limit-atomic reason=deadline"},
		{app.StartupRedisSecurity, "refresh-persistence", "backend-error", "substage=refresh-persistence reason=backend-error"},
		{app.StartupRedisSecurity, "synthetic-secret", "deadline", "substage=unclassified reason=unclassified"},
		{app.StartupRedisSecurity, "rate-limit-atomic", "synthetic-secret", "substage=unclassified reason=unclassified"},
		{app.StartupRedisSecurity, "", "", "substage=unclassified reason=unclassified"},
		{app.StartupRedisPing, "rate-limit-atomic", "deadline", ""},
	} {
		got := redisStartupDiagnosticMessage(tc.stage, tc.substage, tc.reason)
		if tc.want == "" {
			if got != "" {
				t.Fatal("unrelated stage emitted")
			}
			continue
		}
		if !strings.HasSuffix(got, tc.want) || !strings.Contains(got, "code=31") || strings.Contains(got, "synthetic-secret") {
			t.Fatal("unsafe or incorrect event")
		}
	}
}
func TestStartupDiagnosticSink(t *testing.T) {
	msg := redisStartupDiagnosticMessage(app.StartupRedisSecurity, "rate-limit-atomic", "deadline")
	calls := 0
	failure := errors.New("synthetic sink failure")
	ctx := withStartupDiagnosticSink(context.Background(), func(got string) error {
		calls++
		if got != msg {
			t.Fatal("message changed")
		}
		return failure
	})
	if err := emitStartupDiagnostic(context.Background(), msg); err != nil {
		t.Fatal("console emission failed")
	}
	if err := publishRedisStartupDiagnostic(ctx, nil); err != nil {
		t.Fatal("nil error emitted")
	}
	if calls != 0 {
		t.Fatal("unexpected emission")
	}
	if err := emitStartupDiagnostic(ctx, msg); !errors.Is(err, failure) || calls != 1 {
		t.Fatal("sink failure lost")
	}
}
