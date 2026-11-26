package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/example/sistemaemgo/internal/config"
)

func TestStartupStageRedactsCause(t *testing.T) {
	cause := errors.New("synthetic-secret-not-for-logs")
	stages := []StartupStage{StartupConfig, StartupTelemetry, StartupPostgresConfig,
		StartupPostgresPool, StartupPostgresPing, StartupSchema, StartupRedisConfig,
		StartupRedisTLS, StartupRedisTelemetry, StartupRedisPing, StartupRedisSecurity, StartupFiscal}
	for _, stage := range stages {
		t.Run(string(stage), func(t *testing.T) {
			err := startupFailure(stage, cause)
			if strings.Contains(err.Error(), cause.Error()) || !errors.Is(err, cause) {
				t.Fatal("unexpected error disclosure or lost cause")
			}
			if got := StartupStageOf(fmt.Errorf("wrapper: %w", err)); got != stage {
				t.Fatalf("stage=%q want %q", got, stage)
			}
		})
	}
	for _, err := range []error{nil, cause, startupFailure("unapproved", cause)} {
		if StartupStageOf(err) != "" {
			t.Fatal("unknown stage accepted")
		}
	}
}

func TestNewStartupConfigStages(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want StartupStage
	}{
		{"missing database", config.Config{}, StartupConfig},
		{"missing JWT", config.Config{DatabaseURL: "synthetic"}, StartupConfig},
		{"invalid database URL", config.Config{DatabaseURL: "postgres://%zz", JWTSecret: "synthetic"}, StartupPostgresConfig},
		{"invalid telemetry exporter", config.Config{DatabaseURL: "synthetic", JWTSecret: "synthetic", OTelEnabled: true, OTelExporter: "invalid"}, StartupTelemetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New(context.Background(), tc.cfg, slog.Default())
			if a != nil || StartupStageOf(err) != tc.want {
				t.Fatalf("stage=%q want %q", StartupStageOf(err), tc.want)
			}
		})
	}
}
