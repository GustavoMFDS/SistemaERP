package main

import "github.com/example/sistemaemgo/internal/app"

// Stable service-specific diagnostic codes. 10-20 retain their existing
// meanings. Unknown errors retain the legacy application failure code 14.
func appStartupExitCode(stage app.StartupStage) uint32 {
	switch stage {
	case app.StartupConfig:
		return 21
	case app.StartupTelemetry:
		return 22
	case app.StartupPostgresConfig:
		return 23
	case app.StartupPostgresPool:
		return 24
	case app.StartupPostgresPing:
		return 25
	case app.StartupSchema:
		return 26
	case app.StartupRedisConfig:
		return 27
	case app.StartupRedisTLS:
		return 28
	case app.StartupRedisTelemetry:
		return 29
	case app.StartupRedisPing:
		return 30
	case app.StartupRedisSecurity:
		return 31
	case app.StartupFiscal:
		return 32
	default:
		return exitApp
	}
}
