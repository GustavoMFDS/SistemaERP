//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestLocalGarnetRedisSecurityContract(t *testing.T) {
	addr := os.Getenv("TEST_GARNET_ADDR")
	if addr == "" {
		t.Skip("TEST_GARNET_ADDR not set; this security contract must run against a separate Garnet instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: 0})
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Garnet availability: %v", err)
	}

	t.Run("refresh token can be consumed once under concurrency", func(t *testing.T) {
		store := authinfra.NewRefreshTokenStore(client)
		tokenID := "e2e:garnet:" + uuid.NewString()
		userID := uuid.NewString()
		if err := store.Save(ctx, tokenID, userID, 60); err != nil {
			t.Fatalf("Save refresh token: %v", err)
		}
		const workers = 16
		var successful atomic.Int32
		var errs atomic.Int32
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := store.Consume(ctx, tokenID, userID)
				if err != nil {
					errs.Add(1)
				} else if ok {
					successful.Add(1)
				}
			}()
		}
		wg.Wait()
		if errs.Load() != 0 || successful.Load() != 1 {
			t.Fatalf("atomic refresh failed: successes=%d errors=%d; expected exactly one and no errors",
				successful.Load(), errs.Load())
		}
		if ok, err := store.Consume(ctx, tokenID, userID); err != nil || ok {
			t.Fatalf("already consumed token was accepted: ok=%v err=%v", ok, err)
		}
	})

	t.Run("revoke prevents renewal and verifies stored identity", func(t *testing.T) {
		store := authinfra.NewRefreshTokenStore(client)
		tokenID := "e2e:garnet:" + uuid.NewString()
		userID := uuid.NewString()
		if err := store.Save(ctx, tokenID, userID, 60); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.Consume(ctx, tokenID, "wrong-user"); err != nil || ok {
			t.Fatalf("wrong user consumed token: ok=%v err=%v", ok, err)
		}
		if err := store.Revoke(ctx, tokenID); err != nil {
			t.Fatal(err)
		}
		if ok, err := store.Consume(ctx, tokenID, userID); err != nil || ok {
			t.Fatalf("revoked token consumed: ok=%v err=%v", ok, err)
		}
	})

	t.Run("rate limiter is atomic and fails closed without backend", func(t *testing.T) {
		const limit = 5
		const workers = 16
		key := fmt.Sprintf("garnet-%s", uuid.NewString())
		var allowed atomic.Int32
		var failures atomic.Int32
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ok, err := middleware.AllowRateLimit(ctx, client, "garnet_contract", key,
					limit, time.Minute, true)
				if err != nil {
					failures.Add(1)
				} else if ok {
					allowed.Add(1)
				}
			}()
		}
		wg.Wait()
		if allowed.Load() != limit || failures.Load() != 0 {
			t.Fatalf("rate limit is not atomic: allowed=%d want=%d errors=%d",
				allowed.Load(), limit, failures.Load())
		}
		if ok, err := middleware.AllowRateLimit(ctx, nil, "garnet_contract",
			"no-backend", limit, time.Minute, true); ok || !errors.Is(err, middleware.ErrRateLimitUnavailable) {
			t.Fatalf("rate limiter did not fail closed: allowed=%v err=%v", ok, err)
		}
	})

	t.Run("TTL is enforced for one-time refresh tokens", func(t *testing.T) {
		store := authinfra.NewRefreshTokenStore(client)
		tokenID := "e2e:garnet:" + uuid.NewString()
		userID := uuid.NewString()
		if err := store.Save(ctx, tokenID, userID, 1); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1250 * time.Millisecond)
		if ok, err := store.Consume(ctx, tokenID, userID); err != nil || ok {
			t.Fatalf("expired refresh token was accepted: ok=%v err=%v", ok, err)
		}
	})
}
