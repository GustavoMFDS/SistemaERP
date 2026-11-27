package main

import (
	"context"
	"fmt"

	"github.com/example/sistemaemgo/internal/app"
)

type startupDiagnosticSinkKey struct{}
type startupDiagnosticSink func(string) error

// The sink is injected only by native Windows service dispatch.
func withStartupDiagnosticSink(ctx context.Context, sink startupDiagnosticSink) context.Context {
	return context.WithValue(ctx, startupDiagnosticSinkKey{}, sink)
}

func redisStartupDiagnosticMessage(stage app.StartupStage, substage, reason string) string {
	if stage != app.StartupRedisSecurity {
		return ""
	}
	switch substage {
	case "backend", "refresh-persistence", "refresh-user-isolation", "refresh-atomic-consume", "rate-limit-atomic":
	default:
		substage, reason = "unclassified", "unclassified"
	}
	switch reason {
	case "missing", "invariant", "backend-error", "deadline", "canceled", "certificate-validation", "redis-response", "tls-protocol", "transport-transient":
	default:
		substage, reason = "unclassified", "unclassified"
	}
	return fmt.Sprintf("Esterium startup failure: code=31 stage=redis-security substage=%s reason=%s", substage, reason)
}

func emitStartupDiagnostic(ctx context.Context, message string) error {
	if message == "" {
		return nil
	}
	sink, ok := ctx.Value(startupDiagnosticSinkKey{}).(startupDiagnosticSink)
	if !ok || sink == nil {
		return nil
	}
	return sink(message)
}

func publishRedisStartupDiagnostic(ctx context.Context, err error) error {
	substage, reason := app.RedisSecurityDiagnostic(err)
	return emitStartupDiagnostic(ctx, redisStartupDiagnosticMessage(app.StartupStageOf(err), substage, reason))
}
