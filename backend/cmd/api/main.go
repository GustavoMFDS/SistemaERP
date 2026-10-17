package main

import (
	"context"
	"crypto/tls"
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
	checkOnly, argsErr := checkConfigArgument(os.Args[1:])
	if argsErr != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("invalid startup argument", slog.Any("err", argsErr))
		os.Exit(2)
	}
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("invalid configuration", slog.Any("err", err))
		os.Exit(1)
	}
	logger := config.NewLogger(cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	webDir := os.Getenv("ESTERIUM_WEB_DIR")
	certFile := os.Getenv("ESTERIUM_TLS_CERT_FILE")
	keyFile := os.Getenv("ESTERIUM_TLS_KEY_FILE")
	useTLS, tlsErr := resolveLocalTLS(webDir, cfg.HTTPAddr, certFile, keyFile, cfg.AppPublicURL)
	if tlsErr != nil {
		logger.Error("unsafe local server configuration", slog.Any("err", tlsErr))
		os.Exit(1)
	}
	if checkErr := preflightFrontendBundle(webDir); checkErr != nil {
		logger.Error("invalid local frontend bundle", slog.Any("err", checkErr))
		os.Exit(1)
	}
	if checkOnly {
		logger.Info("configuration preflight passed without contacting database or Redis")
		return
	}

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to init app", slog.Any("err", err))
		os.Exit(1)
	}
	defer application.Close()

	handler := application.Router
	if webDir != "" {
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
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS13},
	}

	go func() {
		protocol := "http"
		if useTLS {
			protocol = "https"
		}
		logger.Info("server starting", slog.String("addr", cfg.HTTPAddr), slog.String("protocol", protocol))
		var serveErr error
		if useTLS {
			serveErr = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("server error", slog.Any("err", serveErr))
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
