package app

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

const (
	localStartupPingBudget      = 15 * time.Second
	localRedisStartupPingBudget = 35 * time.Second
	localStartupPingInterval    = 500 * time.Millisecond
)

func localStartupHost(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func localPostgresStartup(cfg *pgconn.Config) bool {
	if cfg == nil || !localStartupHost(cfg.Host) {
		return false
	}
	for _, fallback := range cfg.Fallbacks {
		if fallback == nil || !localStartupHost(fallback.Host) {
			return false
		}
	}
	return true
}

func localRedisStartup(network, addr string) bool {
	if network != "" && network != "tcp" && network != "tcp4" && network != "tcp6" {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	return err == nil && localStartupHost(host)
}

// Every joined error leaf must be explicitly transient. A refused fallback
// cannot mask an authentication or TLS error.
func startupPingTransient(err error, postgres bool) bool {
	if err == nil {
		return false
	}
	// Never turn a cancellation, server response or certificate/protocol
	// validation failure into an eligible transport retry.
	if !postgres {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return false
		}
		switch err.(type) {
		case redis.Error, *tls.CertificateVerificationError, tls.RecordHeaderError, *tls.RecordHeaderError:
			return false
		}
	}
	if pg, ok := err.(*pgconn.PgError); ok {
		return postgres && pg.Code == "57P03"
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		children := multi.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !startupPingTransient(child, postgres) {
				return false
			}
		}
		return true
	}
	if child := errors.Unwrap(err); child != nil {
		return startupPingTransient(child, postgres)
	}
	if errno, ok := err.(syscall.Errno); ok {
		if errno == syscall.ECONNREFUSED || (runtime.GOOS == "windows" && errno == syscall.Errno(10061)) {
			return true
		}
		return !postgres && (errno == syscall.ETIMEDOUT || (runtime.GOOS == "windows" && errno == syscall.Errno(10060)))
	}
	// Only typed network timeouts, and only for local Redis startup. Do
	// not infer transient failures from strings or retry other errors.
	if !postgres {
		if transport, ok := err.(net.Error); ok {
			return transport.Timeout()
		}
	}
	return false
}

// Only Ping is retried, never migrations, security probes or application writes.
func waitForStartupPing(parent context.Context, budget, interval time.Duration, postgres bool, ping func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := ping(ctx)
		if err == nil {
			return ctx.Err()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !startupPingTransient(err, postgres) {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
