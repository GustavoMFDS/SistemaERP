package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// CheckLocalDependencies verifies the live PostgreSQL schema and real Redis
// atomic security contract without serving requests, starting background
// workers, applying migrations, reading business data or contacting SEFAZ.
// The Redis check writes only uniquely named short-lived synthetic keys.
func CheckLocalDependencies(parent context.Context, cfg config.Config) error {
	if cfg.DatabaseURL == "" {
		return errors.New("PostgreSQL connection is not configured")
	}
	if cfg.DisableRedis {
		return errors.New("Redis security backend must be enabled for the readiness check")
	}
	if cfg.RedisURL == "" && cfg.RedisAddr == "" {
		return errors.New("Redis security backend is not configured")
	}

	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	pgConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("PostgreSQL connection configuration is invalid")
	}
	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return errors.New("PostgreSQL pool could not be initialized")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("PostgreSQL is unavailable or its TLS identity is untrusted: %w", err)
	}
	var version int
	var dirty bool
	if err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty); err != nil {
		return fmt.Errorf("PostgreSQL migrations table is unavailable: %w", err)
	}
	if version <= 0 || dirty {
		return fmt.Errorf("PostgreSQL migrations are incomplete or dirty (version=%d, dirty=%v)", version, dirty)
	}
	dir := strings.TrimSpace(os.Getenv("ESTERIUM_MIGRATIONS_DIR"))
	if dir == "" && cfg.IsProdLike() && strings.TrimSpace(os.Getenv("ESTERIUM_WEB_DIR")) != "" {
		return errors.New("local installation requires ESTERIUM_MIGRATIONS_DIR to verify the installed schema version")
	}
	if dir != "" {
		bundledVersion, err := localMigrationHead(dir)
		if err != nil {
			return fmt.Errorf("SQL migration bundle is invalid: %w", err)
		}
		if version != bundledVersion {
			return fmt.Errorf("database schema version %d differs from bundled release version %d; migration review required", version, bundledVersion)
		}
	}
	// Schema-only checks: do not select customer, financial or fiscal contents.
	required := []string{
		"companies", "users", "products", "sales", "sale_items", "payments",
		"audit_logs", "nfce_configs", "fiscal_document_sequences",
		"sale_fiscal_intents", "schema_migrations",
	}
	var present int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name=ANY($1)`, required).Scan(&present); err != nil {
		return fmt.Errorf("PostgreSQL required schema inspection failed: %w", err)
	}
	if present != len(required) {
		return fmt.Errorf("PostgreSQL schema is incomplete: %d of %d critical tables found", present, len(required))
	}

	var options *redis.Options
	if cfg.RedisURL != "" {
		options, err = redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return errors.New("Redis TLS connection configuration is invalid")
		}
	} else {
		options = &redis.Options{
			Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB,
		}
	}
	client := redis.NewClient(options)
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis is unavailable or its TLS identity is untrusted: %w", err)
	}
	if err := verifyRedisSecurityPrimitives(ctx, client); err != nil {
		return fmt.Errorf("Redis atomic session and rate-limit contract rejected: %w", err)
	}
	return nil
}
