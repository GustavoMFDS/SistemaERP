package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/example/sistemaemgo/internal/platform/observability"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

type App struct {
	DB                *pgxpool.Pool
	Redis             *redis.Client
	Router            http.Handler
	telemetryShutdown func(context.Context) error
	logger            *slog.Logger
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}

	telemetryShutdown, err := observability.Setup(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		_ = telemetryShutdown(ctx)
		return nil, err
	}
	if cfg.OTelEnabled {
		poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_ = telemetryShutdown(ctx)
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = telemetryShutdown(ctx)
		return nil, err
	}

	var rdb *redis.Client
	if !cfg.DisableRedis {
		var opts *redis.Options
		if cfg.RedisURL != "" {
			opts, err = redis.ParseURL(cfg.RedisURL)
			if err != nil {
				pool.Close()
				_ = telemetryShutdown(ctx)
				return nil, err
			}
		} else {
			opts = &redis.Options{
				Addr:     cfg.RedisAddr,
				Password: cfg.RedisPassword,
				DB:       cfg.RedisDB,
			}
		}
		rdb = redis.NewClient(opts)
		if cfg.OTelEnabled {
			if err := redisotel.InstrumentTracing(rdb); err != nil {
				pool.Close()
				_ = rdb.Close()
				_ = telemetryShutdown(ctx)
				return nil, err
			}
		}
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := rdb.Ping(pingCtx).Err(); err != nil {
			pool.Close()
			_ = rdb.Close()
			_ = telemetryShutdown(ctx)
			return nil, err
		}
	} else if cfg.IsProdLike() {
		pool.Close()
		_ = telemetryShutdown(ctx)
		return nil, errors.New("redis is required in staging/prod because refresh token rotation depends on it")
	} else {
		logger.Warn("redis_disabled_dev_mode", slog.String("impact", "refresh tokens and optional caches are unavailable"))
	}

	mods := modules.New(cfg, pool, rdb, logger)
	router := httpapi.NewRouter(cfg, mods, logger)

	return &App{DB: pool, Redis: rdb, Router: router, telemetryShutdown: telemetryShutdown, logger: logger}, nil
}

func (a *App) Close() {
	if a.telemetryShutdown != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.telemetryShutdown(shutdownCtx)
	}
	if a.DB != nil {
		a.DB.Close()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
}
