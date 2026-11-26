package app

import "errors"

// StartupStage is a fixed diagnostic label. It contains no configuration,
// connection string, credential, filesystem path or raw database response.
type StartupStage string

const (
	StartupConfig         StartupStage = "config"
	StartupTelemetry      StartupStage = "telemetry"
	StartupPostgresConfig StartupStage = "postgres-config"
	StartupPostgresPool   StartupStage = "postgres-pool"
	StartupPostgresPing   StartupStage = "postgres-ping"
	StartupSchema         StartupStage = "schema"
	StartupRedisConfig    StartupStage = "redis-config"
	StartupRedisTLS       StartupStage = "redis-tls"
	StartupRedisTelemetry StartupStage = "redis-telemetry"
	StartupRedisPing      StartupStage = "redis-ping"
	StartupRedisSecurity  StartupStage = "redis-security"
	StartupFiscal         StartupStage = "fiscal"
)

type startupError struct {
	stage StartupStage
	cause error
}

func (e *startupError) Error() string { return "application startup failed: " + string(e.stage) }
func (e *startupError) Unwrap() error { return e.cause }
func startupFailure(stage StartupStage, cause error) error {
	return &startupError{stage: stage, cause: cause}
}

// StartupStageOf returns only an allowlisted diagnostic stage.
func StartupStageOf(err error) StartupStage {
	var e *startupError
	if errors.As(err, &e) && e != nil {
		switch e.stage {
		case StartupConfig, StartupTelemetry, StartupPostgresConfig, StartupPostgresPool,
			StartupPostgresPing, StartupSchema, StartupRedisConfig, StartupRedisTLS,
			StartupRedisTelemetry, StartupRedisPing, StartupRedisSecurity, StartupFiscal:
			return e.stage
		}
	}
	return ""
}
