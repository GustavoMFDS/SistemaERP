package config

import (
	"strings"
	"testing"
	"time"
)

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
