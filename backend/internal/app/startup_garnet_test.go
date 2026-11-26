package app

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// A real go-redis client encounters a listening but nonresponsive local server,
// then an authenticated RESP server. No production data or external network.
func TestGarnetStartupDelayedResponse(t *testing.T) {
	testGarnetStartupResponse(t, false)
}

func TestGarnetStartupRejectsAuthentication(t *testing.T) {
	testGarnetStartupResponse(t, true)
}

func testGarnetStartupResponse(t *testing.T, rejectAuth bool) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			first := accepted.Add(1) == 1
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				if first && !rejectAuth {
					<-stop
					return
				}
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				rd := bufio.NewReader(conn)
				authenticated := false
				for {
					line, err := rd.ReadString('\n')
					if err != nil {
						return
					}
					if !strings.HasPrefix(line, "*") {
						return
					}
					n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
					if err != nil || n < 1 || n > 16 {
						return
					}
					args := make([]string, n)
					for i := range args {
						line, err = rd.ReadString('\n')
						if err != nil || !strings.HasPrefix(line, "$") {
							return
						}
						size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
						if err != nil || size < 0 || size > 1024 {
							return
						}
						data := make([]byte, size+2)
						if _, err = io.ReadFull(rd, data); err != nil {
							return
						}
						args[i] = string(data[:size])
					}
					response := "+OK\r\n"
					switch strings.ToUpper(args[0]) {
					case "HELLO":
						response = "-ERR unknown command 'hello'\r\n"
					case "AUTH":
						if !rejectAuth && len(args) == 3 && args[1] == "synthetic" && args[2] == "synthetic-password" {
							authenticated = true
						} else {
							response = "-WRONGPASS invalid username-password pair\r\n"
						}
					case "PING":
						if authenticated {
							response = "+PONG\r\n"
						} else {
							response = "-NOAUTH Authentication required\r\n"
						}
					}
					if _, err = io.WriteString(conn, response); err != nil {
						return
					}
				}
			}()
		}
	}()
	defer func() { ln.Close(); <-done; close(stop); wg.Wait() }()
	client := redis.NewClient(&redis.Options{Addr: ln.Addr().String(), Username: "synthetic", Password: "synthetic-password", Protocol: 2, DisableIdentity: true, MaxRetries: -1, DialerRetries: 1, ReadTimeout: 80 * time.Millisecond, WriteTimeout: 80 * time.Millisecond, DialTimeout: time.Second, ContextTimeoutEnabled: true})
	defer client.Close()
	calls := 0
	err = waitForStartupPing(context.Background(), 2*time.Second, 10*time.Millisecond, false, func(ctx context.Context) error {
		calls++
		return client.Ping(ctx).Err()
	})
	if rejectAuth {
		var authErr redis.Error
		if !errors.As(err, &authErr) || calls != 1 || redisStartupPingFailureKind(err) != "redis-response" {
			t.Fatalf("authentication must fail without outer retry: calls=%d err=%v", calls, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("startup did not recover from delayed local server: %v", err)
	}
	if accepted.Load() < 2 {
		t.Fatal("fixture did not exercise a reconnect")
	}
}

func TestGarnetStartupTransportClassification(t *testing.T) {
	timeout := &net.OpError{Op: "read", Net: "tcp", Err: timeoutFixture{}}
	cert := &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{timeout, true}, {fmt.Errorf("wrapped: %w", timeout), true},
		{errors.Join(timeout, cert), false}, {cert, false},
		{context.DeadlineExceeded, false}, {context.Canceled, false},
		{errors.New("i/o timeout"), false},
	} {
		if got := startupPingTransient(tc.err, false); got != tc.want {
			t.Errorf("%T retry=%v want %v", tc.err, got, tc.want)
		}
		if startupPingTransient(tc.err, true) {
			t.Errorf("PostgreSQL behavior unexpectedly changed for %T", tc.err)
		}
	}
}

type timeoutFixture struct{}

func (timeoutFixture) Error() string   { return "synthetic transport timeout" }
func (timeoutFixture) Timeout() bool   { return true }
func (timeoutFixture) Temporary() bool { return true }

func TestGarnetStartupTimeoutBudget(t *testing.T) {
	calls := 0
	err := waitForStartupPing(context.Background(), 60*time.Millisecond, 5*time.Millisecond, false, func(context.Context) error {
		calls++
		return timeoutFixture{}
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls == 0 {
		t.Fatalf("continuous timeout must exhaust bounded startup budget: calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls = 0
	err = waitForStartupPing(ctx, time.Second, time.Millisecond, false, func(context.Context) error {
		calls++
		cancel()
		return timeoutFixture{}
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancellation must terminate retries: calls=%d err=%v", calls, err)
	}
}
