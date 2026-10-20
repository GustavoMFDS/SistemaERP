package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvReadsProtectedDatabaseAndRedisFiles(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("JWT_SECRET", "safe-local-integration-secret-of-ample-length")
	t.Setenv("FISCAL_PROVIDER", "disabled")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database-url.secret")
	redisPath := filepath.Join(dir, "redis-url.secret")
	wantDB := "postgres://app:integration-db-password@localhost:5432/esterium?sslmode=disable"
	wantRedis := "redis://:integration-redis-password@localhost:6379/0"
	if err := os.WriteFile(dbPath, []byte(wantDB+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(redisPath, []byte(wantRedis+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL_FILE", dbPath)
	t.Setenv("REDIS_URL_FILE", redisPath)
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("read protected local URLs: %v", err)
	}
	if cfg.DatabaseURL != wantDB || cfg.RedisURL != wantRedis {
		t.Fatalf("connection URLs were not read from protected files")
	}
}

func TestLoadFromEnvReadsMetricsBearerTokenFile(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("METRICS_BEARER_TOKEN", "")
	file := filepath.Join(t.TempDir(), "metrics-token.secret")
	const token = "not-an-actual-operational-token"
	if err := os.WriteFile(file, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METRICS_BEARER_TOKEN_FILE", file)
	// This focused test validates the secret-file reader directly, without
	// implying an entire database/Redis stack is provisioned.
	got, err := getEnvOrFile("METRICS_BEARER_TOKEN", "METRICS_BEARER_TOKEN_FILE")
	if err != nil || got != token {
		t.Fatalf("metrics token file not loaded: got expected=%v err=%v", got == token, err)
	}
}

func TestLoadFromEnvRejectsMissingProtectedURLFile(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_FILE", filepath.Join(t.TempDir(), "missing.secret"))
	t.Setenv("JWT_SECRET", "safe-local-integration-secret-of-ample-length")
	_, err := LoadFromEnv()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL_FILE") {
		t.Fatalf("missing database file was not rejected: %v", err)
	}
}

func TestValidateRejectsWildcardCORS(t *testing.T) {
	cfg := validProdConfig()
	cfg.CORSAllowedOrigins = []string{"*"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard CORS validation error, got %v", err)
	}
}

func TestValidateRejectsLongAccessTokenTTLInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.AccessTokenTTL = 30 * time.Minute

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ACCESS_TOKEN_TTL_MINUTES") {
		t.Fatalf("expected access token TTL validation error, got %v", err)
	}
}

func TestValidateAllowsDevelopmentLocalhostCORS(t *testing.T) {
	cfg := validProdConfig()
	cfg.Env = "dev"
	cfg.CORSAllowedOrigins = []string{"http://localhost:5173"}
	cfg.MetricsBearerToken = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected development localhost CORS config to validate: %v", err)
	}
}

func TestValidateRejectsDisabledRedisInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.DisableRedis = true

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DISABLE_REDIS") {
		t.Fatalf("expected disabled Redis validation error, got %v", err)
	}
}

func TestValidateAcceptsRedisURL(t *testing.T) {
	cfg := validProdConfig()
	cfg.RedisAddr = ""
	cfg.RedisURL = "rediss://:strong-redis-password@redis.example.internal:6379/0"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected REDIS_URL config to validate: %v", err)
	}
}

func TestValidateRejectsInvalidRedisURL(t *testing.T) {
	cfg := validProdConfig()
	cfg.RedisURL = "http://redis.example.internal:6379"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("expected REDIS_URL validation error, got %v", err)
	}
}

func TestValidateRejectsDatabaseWithoutVerifyFullInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.DatabaseURL = "postgres://app:super-secret-password@db:5432/sistemaemgo?sslmode=require"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "sslmode=verify-full") {
		t.Fatalf("expected verify-full database validation error, got %v", err)
	}
}

func TestValidateRejectsPlainRedisInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.RedisURL = "redis://:strong-redis-password@redis.example.internal:6379/0"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "rediss://") {
		t.Fatalf("expected rediss validation error, got %v", err)
	}
}

func TestValidateRejectsSplitRedisInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.RedisURL = ""
	cfg.RedisAddr = "redis.example.internal:6379"
	cfg.RedisPassword = "strong-redis-password"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "REDIS_URL with rediss://") {
		t.Fatalf("expected production REDIS_URL requirement, got %v", err)
	}
}

func TestValidateRejectsMVPFiscalProviderInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.FiscalProvider = "mvp"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "not SEFAZ-ready") {
		t.Fatalf("expected production fiscal provider validation error, got %v", err)
	}
}

func TestValidateRequiresExplicitProductionSEFAZGate(t *testing.T) {
	cfg := validProdConfig()
	cfg.FiscalProvider = "sefaz"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "explicit SEFAZ transmission environment flag") {
		t.Fatalf("expected explicit production SEFAZ gate error, got %v", err)
	}
}

func TestValidateAcceptsExplicitProductionSEFAZConfiguration(t *testing.T) {
	cfg := validProdConfig()
	cfg.FiscalProvider = "sefaz"
	cfg.NFCeSEFAZProductionEnabled = true
	cfg.NFCeCertificateSecretDir = "/run/secrets/nfce"
	cfg.NFCeSchemaDir = "/opt/nfce/schema"
	cfg.NFCeSchemaEntrypoint = "leiauteNFe_v4.00.xsd"
	cfg.NFCeEventSchemaEntrypoint = "envEventoCancNFe_v1.00.xsd"
	cfg.NFCeInutilizationSchemaEntrypoint = "inutNFe_v4.00.xsd"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected explicit production SEFAZ config to validate: %v", err)
	}
}

func TestValidateRejectsProductionSEFAZGateOutsideProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.Env = "staging"
	cfg.FiscalProvider = "sefaz"
	cfg.NFCeSEFAZProductionEnabled = true
	cfg.NFCeCertificateSecretDir = "/run/secrets/nfce"
	cfg.NFCeSchemaDir = "/opt/nfce/schema"
	cfg.NFCeSchemaEntrypoint = "leiauteNFe_v4.00.xsd"
	cfg.NFCeEventSchemaEntrypoint = "envEventoCancNFe_v1.00.xsd"
	cfg.NFCeInutilizationSchemaEntrypoint = "inutNFe_v4.00.xsd"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "only allowed in production") {
		t.Fatalf("expected production SEFAZ environment validation error, got %v", err)
	}
}

func TestValidateRejectsPlaceholderSecretsInProd(t *testing.T) {
	cfg := validProdConfig()
	cfg.JWTSecret = "REPLACE_WITH_RANDOM_32_PLUS_CHARACTER_SECRET"
	cfg.DatabaseURL = "postgres://app:REPLACE_WITH_STRONG_DB_PASSWORD@db:5432/sistemaemgo?sslmode=require"

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "placeholder") || !strings.Contains(err.Error(), "DATABASE_URL password") {
		t.Fatalf("expected placeholder secret validation errors, got %v", err)
	}
}

func validProdConfig() Config {
	return Config{
		Env:                "prod",
		ServiceName:        "sistemaemgo-api",
		HTTPAddr:           ":8080",
		DatabaseURL:        "postgres://app:super-secret-password@db:5432/sistemaemgo?sslmode=verify-full",
		RedisURL:           "rediss://:super-secret-redis-password@redis.example.internal:6379/0",
		JWTSecret:          "this-is-a-very-long-production-jwt-secret",
		JWTIssuer:          "sistemaemgo",
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		CORSAllowedOrigins: []string{"https://app.example.com"},
		CORSAllowedMethods: []string{"GET", "POST"},
		CORSAllowedHeaders: []string{"Authorization", "Content-Type"},
		MetricsBearerToken: "metrics-secret",
		RateLimitLogin:     10,
		RateLimitLoginID:   5,
		RateLimitLoginIPID: 5,
		RateLimitRefresh:   30,
		RateLimitLogout:    30,
		RateLimitSales:     60,
		RateLimitFiscal:    20,
		FiscalProvider:     "disabled",
	}
}
