package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// verifyRedisSecurityPrimitives is a fail-closed production startup gate.
// PING alone is insufficient: session revocation and rate limiting require
// actual atomic Lua EVAL support from any Redis-compatible backend.
func verifyRedisSecurityPrimitives(parent context.Context, client *redis.Client) error {
	if client == nil {
		return fmt.Errorf("redis security backend is missing")
	}
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()

	store := authinfra.NewRefreshTokenStore(client)
	tokenID := "esterium:startup:" + uuid.NewString()
	userID := uuid.NewString()
	if err := store.Save(ctx, tokenID, userID, 20); err != nil {
		return fmt.Errorf("refresh persistence unavailable: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = store.Revoke(cleanupCtx, tokenID)
	}()

	if ok, err := store.Consume(ctx, tokenID, "invalid-user"); err != nil || ok {
		return fmt.Errorf("refresh user isolation failed: accepted=%v, err=%v", ok, err)
	}

	const concurrentCalls = 4
	var consumed atomic.Int32
	var errorsSeen atomic.Int32
	var wg sync.WaitGroup
	for range concurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.Consume(ctx, tokenID, userID)
			if err != nil {
				errorsSeen.Add(1)
			} else if ok {
				consumed.Add(1)
			}
		}()
	}
	wg.Wait()
	if errorsSeen.Load() != 0 || consumed.Load() != 1 {
		return fmt.Errorf("refresh consume was not atomic (consumed=%d errors=%d)",
			consumed.Load(), errorsSeen.Load())
	}

	const rateLimit = 2
	var allowed atomic.Int32
	errorsSeen.Store(0)
	key := "esterium:startup:" + uuid.NewString()
	for range concurrentCalls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := middleware.AllowRateLimit(ctx, client, "startup_contract",
				key, rateLimit, time.Minute, true)
			if err != nil {
				errorsSeen.Add(1)
			} else if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if errorsSeen.Load() != 0 || allowed.Load() != rateLimit {
		return fmt.Errorf("rate-limit Lua command was not atomic (allowed=%d errors=%d)",
			allowed.Load(), errorsSeen.Load())
	}
	return nil
}
