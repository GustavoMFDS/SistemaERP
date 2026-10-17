package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/sistemaemgo/internal/app"
	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("invalid configuration", slog.Any("err", err))
		os.Exit(1)
	}
	logger := config.NewLogger(cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to init app", slog.Any("err", err))
		os.Exit(1)
	}
	defer application.Close()

	handler := application.Router
	if webDir := os.Getenv("ESTERIUM_WEB_DIR"); webDir != "" {
		handler, err = httpapi.WithLocalFrontend(handler, webDir)
		if err != nil {
			logger.Error("invalid local frontend bundle", slog.Any("err", err))
			os.Exit(1)
		}
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           wrapTelemetryHandler(cfg, handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("http server starting", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", slog.Any("err", err))
			cancel()
		}
	}()

	// When started by the Windows Service Control Manager, react to Stop and
	// Shutdown requests through its native control channel. Console usage and
	// Linux still use normal process signals with no service dependency.
	if isService, serviceErr := maybeRunWindowsService(ctx, cancel); serviceErr != nil {
		logger.Error("windows service dispatcher failed", slog.Any("err", serviceErr))
		cancel()
	} else if isService {
		cancel()
	}

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	logger.Info("shutdown complete")
}

func wrapTelemetryHandler(cfg config.Config, h http.Handler) http.Handler {
	if !cfg.OTelEnabled {
		return h
	}
	return otelhttp.NewHandler(h, cfg.ServiceName)
}
