package main

import (
	"testing"

	"github.com/example/sistemaemgo/internal/app"
)

func TestAppStartupExitCodes(t *testing.T) {
	cases := map[app.StartupStage]uint32{
		app.StartupConfig: 21, app.StartupTelemetry: 22, app.StartupPostgresConfig: 23,
		app.StartupPostgresPool: 24, app.StartupPostgresPing: 25, app.StartupSchema: 26,
		app.StartupRedisConfig: 27, app.StartupRedisTLS: 28, app.StartupRedisTelemetry: 29,
		app.StartupRedisPing: 30, app.StartupRedisSecurity: 31, app.StartupFiscal: 32,
		"": 14, "unapproved": 14,
	}
	for stage, want := range cases {
		if got := appStartupExitCode(stage); got != want {
			t.Fatalf("stage=%q code=%d want %d", stage, got, want)
		}
	}
}
