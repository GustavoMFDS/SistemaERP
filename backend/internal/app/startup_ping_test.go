package app

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

func TestStartupPingTransient(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	auth := &pgconn.PgError{Code: "28P01"}
	tls := x509.UnknownAuthorityError{}
	cases := []struct {
		name     string
		err      error
		pg, want bool
	}{
		{"refused", refused, true, true},
		{"wrapped", fmt.Errorf("wrapper: %w", refused), false, true},
		{"postgres starting", &pgconn.PgError{Code: "57P03"}, true, true},
		{"postgres code not Redis", &pgconn.PgError{Code: "57P03"}, false, false},
		{"all transient", errors.Join(refused, &pgconn.PgError{Code: "57P03"}), true, true},
		{"mixed auth", errors.Join(refused, auth), true, false},
		{"mixed TLS", errors.Join(refused, tls), true, false},
		{"nested mixed", fmt.Errorf("wrapper: %w", errors.Join(refused, errors.Join(refused, tls))), true, false},
		{"auth", auth, true, false},
		{"TLS", tls, false, false},
		{"canceled", context.Canceled, true, false},
		{"deadline", context.DeadlineExceeded, true, false},
		{"unknown", errors.New("connection refused"), true, false},
		{"nil", nil, true, false},
		{"winsock", syscall.Errno(10061), true, runtime.GOOS == "windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := startupPingTransient(tc.err, tc.pg); got != tc.want {
				t.Fatalf("retry=%v want %v", got, tc.want)
			}
		})
	}
}

func TestLocalStartupEndpoints(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{{"127.0.0.1", true}, {"::1", true}, {"192.168.0.1", false}, {"localhost", true}, {"example.invalid", false}, {"", false}} {
		if got := localStartupHost(tc.host); got != tc.want {
			t.Fatalf("host %q eligibility=%v", tc.host, got)
		}
	}
	if !localPostgresStartup(&pgconn.Config{Host: "127.0.0.1", Fallbacks: []*pgconn.FallbackConfig{{Host: "::1"}}}) {
		t.Fatal("local fallback rejected")
	}
	if localPostgresStartup(&pgconn.Config{Host: "127.0.0.1", Fallbacks: []*pgconn.FallbackConfig{{Host: "192.168.0.1"}}}) {
		t.Fatal("remote fallback accepted")
	}
	if localPostgresStartup(nil) {
		t.Fatal("nil config accepted")
	}
	for _, tc := range []struct {
		network, addr string
		want          bool
	}{{"tcp", "127.0.0.1:6379", true}, {"", "[::1]:6379", true}, {"unix", "127.0.0.1:6379", false}, {"tcp", "localhost:6379", true}, {"tcp", "1.2.3.4:6379", false}, {"tcp", "invalid", false}} {
		if got := localRedisStartup(tc.network, tc.addr); got != tc.want {
			t.Fatalf("address %q eligibility=%v", tc.addr, got)
		}
	}
}

func TestWaitForStartupPing(t *testing.T) {
	t.Run("eventual success", func(t *testing.T) {
		calls := 0
		err := waitForStartupPing(context.Background(), time.Second, time.Millisecond, true, func(ctx context.Context) error {
			calls++
			if calls < 3 {
				return syscall.ECONNREFUSED
			}
			return nil
		})
		if err != nil || calls != 3 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
	t.Run("permanent failure", func(t *testing.T) {
		calls := 0
		auth := &pgconn.PgError{Code: "28P01"}
		err := waitForStartupPing(context.Background(), time.Second, time.Millisecond, true, func(ctx context.Context) error { calls++; return auth })
		if err != auth || calls != 1 {
			t.Fatal("permanent failure was retried or lost")
		}
	})
	t.Run("expired", func(t *testing.T) {
		calls := 0
		err := waitForStartupPing(context.Background(), 30*time.Millisecond, time.Second, false, func(ctx context.Context) error { calls++; return syscall.ECONNREFUSED })
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	})
	t.Run("canceled before ping", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := waitForStartupPing(ctx, time.Second, time.Millisecond, false, func(context.Context) error { t.Fatal("unexpected ping"); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost")
		}
	})
	t.Run("cancel during ping", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		err := waitForStartupPing(ctx, time.Second, time.Millisecond, false, func(context.Context) error { calls++; cancel(); return nil })
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatal("canceled success was accepted")
		}
	})
}

func TestProvisionedLocalStartupURLs(t *testing.T) {
	pg, err := pgconn.ParseConfig("postgres://esterium_app:synthetic@localhost:5432/esterium?sslmode=verify-full")
	if err != nil || !localPostgresStartup(pg) {
		t.Fatal("provisioned PostgreSQL host not eligible")
	}
	opts, err := redis.ParseURL("rediss://esteriumcache:synthetic@localhost:6379/0")
	if err != nil || !localRedisStartup(opts.Network, opts.Addr) {
		t.Fatal("provisioned Garnet host not eligible")
	}
}
