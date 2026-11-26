package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestGarnetStartupTLSContextDeadline(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	opts := &redis.Options{TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialTimeout: 3 * time.Second}
	configureLocalRedisStartupDialer(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	conn, err := opts.Dialer(ctx, "tcp", ln.Addr().String())
	if conn != nil {
		conn.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("TLS handshake did not honor request deadline: %v", err)
	}
	if startupPingTransient(err, false) || redisStartupPingFailureKind(err) != "deadline" {
		t.Fatal("deadline must not become a retryable transport failure")
	}
}

func TestGarnetStartupTLSIdentity(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	for _, tc := range []struct {
		name  string
		roots *x509.CertPool
		host  string
		valid bool
	}{
		{"pinned", roots, "127.0.0.1", true},
		{"wrong-host", roots, "unrelated.invalid", false},
		{"untrusted", x509.NewCertPool(), "127.0.0.1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &tls.Config{RootCAs: tc.roots, ServerName: tc.host, MinVersion: tls.VersionTLS12}
			opts := &redis.Options{TLSConfig: cfg, DialTimeout: time.Second}
			configureLocalRedisStartupDialer(opts)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := opts.Dialer(ctx, "tcp", server.Listener.Addr().String())
			if conn != nil {
				conn.Close()
			}
			if tc.valid {
				if err != nil {
					t.Fatalf("verified TLS rejected: %v", err)
				}
			} else {
				var cert *tls.CertificateVerificationError
				if !errors.As(err, &cert) || startupPingTransient(err, false) || redisStartupPingFailureKind(err) != "certificate-validation" {
					t.Fatalf("invalid certificate must fail closed: %v", err)
				}
			}
			if opts.TLSConfig != cfg || cfg.InsecureSkipVerify {
				t.Fatal("TLS policy changed")
			}
		})
	}
}

func TestGarnetStartupDialerPreservesExplicitPolicy(t *testing.T) {
	sentinel := errors.New("custom dialer")
	opts := &redis.Options{TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, Dialer: func(context.Context, string, string) (net.Conn, error) { return nil, sentinel }}
	configureLocalRedisStartupDialer(opts)
	if _, err := opts.Dialer(context.Background(), "tcp", "127.0.0.1:1"); err != sentinel {
		t.Fatal("explicit dialer was replaced")
	}
	plain := &redis.Options{}
	configureLocalRedisStartupDialer(plain)
	if plain.Dialer != nil {
		t.Fatal("non-TLS default was replaced")
	}
}
