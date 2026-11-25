package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
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

// Fixed service-specific exit codes distinguish startup stages without sending
// configuration values, credentials or arbitrary error text to the SCM.
const (
	exitConfig uint32 = 10 + iota
	exitTLS
	exitFrontend
	exitDependencies
	exitApp
	exitListen
	exitServe
	exitShutdown
	exitStartupTimeout
	exitShutdownTimeout
	exitUnexpectedStop
)

type serverRun func(context.Context, func()) uint32

func main() {
	mode, argsErr := parseStartupMode(os.Args[1:])
	if argsErr != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("invalid startup argument", slog.Any("err", argsErr))
		os.Exit(2)
	}

	// Connect to SCM before loading configuration, contacting dependencies or
	// starting the server. Preflight flags remain ordinary console processes.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	run := func(ctx context.Context, ready func()) uint32 {
		return runServer(ctx, mode, ready)
	}
	if mode == startupServe {
		if isService, err := maybeRunWindowsService(ctx, run); err != nil {
			slog.Error("windows service dispatcher failed")
			os.Exit(1)
		} else if isService {
			return
		}
	}
	if run(ctx, func() {}) != 0 {
		os.Exit(1)
	}
}

func runServer(parent context.Context, mode startupMode, ready func()) uint32 {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		logger.Error("invalid configuration", slog.Any("err", err))
		return exitConfig
	}
	logger := config.NewLogger(cfg)
	webDir := os.Getenv("ESTERIUM_WEB_DIR")
	certFile := os.Getenv("ESTERIUM_TLS_CERT_FILE")
	keyFile := os.Getenv("ESTERIUM_TLS_KEY_FILE")
	useTLS, tlsErr := resolveLocalTLS(webDir, cfg.HTTPAddr, certFile, keyFile, cfg.AppPublicURL)
	if tlsErr != nil {
		logger.Error("unsafe local server configuration", slog.Any("err", tlsErr))
		return exitTLS
	}
	if checkErr := preflightFrontendBundle(webDir); checkErr != nil {
		logger.Error("invalid local frontend bundle", slog.Any("err", checkErr))
		return exitFrontend
	}
	if mode == startupConfigCheck {
		logger.Info("configuration preflight passed without contacting database or Redis")
		return 0
	}
	if mode == startupDependencyCheck {
		if depErr := app.CheckLocalDependencies(ctx, cfg); depErr != nil {
			logger.Error("local dependency check failed", slog.Any("err", depErr))
			return exitDependencies
		}
		logger.Info("PostgreSQL schema and Redis atomic security checks passed; server not started")
		return 0
	}

	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to init app", slog.Any("err", err))
		return exitApp
	}
	defer func() {
		cancel()
		application.Close()
	}()

	handler := application.Router
	if webDir != "" {
		handler, err = httpapi.WithLocalFrontend(handler, webDir)
		if err != nil {
			logger.Error("invalid local frontend bundle", slog.Any("err", err))
			return exitFrontend
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
	protocol := "http"
	if useTLS {
		protocol = "https"
		pair, pairErr := tls.LoadX509KeyPair(certFile, keyFile)
		if pairErr != nil {
			logger.Error("TLS key pair could not be loaded", slog.Any("err", pairErr))
			return exitTLS
		}
		srv.TLSConfig.Certificates = []tls.Certificate{pair}
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("server listener failed", slog.Any("err", err))
		return exitListen
	}
	defer listener.Close()
	if ctx.Err() != nil {
		return 0
	}
	logger.Info("server starting", slog.String("addr", cfg.HTTPAddr), slog.String("protocol", protocol))
	// SCM Running means application initialization, TLS key loading and bind
	// succeeded. The installer must still verify trusted HTTPS /health/ready.
	ready()
	served := make(chan error, 1)
	go func() {
		if useTLS {
			served <- srv.ServeTLS(listener, "", "")
		} else {
			served <- srv.Serve(listener)
		}
	}()
	select {
	case serveErr := <-served:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("server error", slog.Any("err", serveErr))
			return exitServe
		}
		return 0
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			logger.Error("server shutdown failed")
			return exitShutdown
		}
		logger.Info("shutdown complete")
		return 0
	}
}

func wrapTelemetryHandler(cfg config.Config, h http.Handler) http.Handler {
	if !cfg.OTelEnabled {
		return h
	}
	return otelhttp.NewHandler(h, cfg.ServiceName)
}
