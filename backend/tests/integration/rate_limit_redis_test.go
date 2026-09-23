//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/redis/go-redis/v9"
)

func TestRedisRateLimitIsAtomicAndKeepsTTL(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not configured")
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	name := "integration_atomic"
	key := "rate-limit-key"
	redisKey := "rl:" + name + ":" + key
	_ = rdb.Del(ctx, redisKey).Err()
	t.Cleanup(func() { _ = rdb.Del(context.Background(), redisKey).Err() })

	for i := 0; i < 2; i++ {
		allowed, err := middleware.AllowRateLimit(ctx, rdb, name, key, 2, 10*time.Second, true)
		if err != nil {
			t.Fatalf("AllowRateLimit attempt %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("attempt %d unexpectedly denied", i+1)
		}
	}

	ttl, err := rdb.PTTL(ctx, redisKey).Result()
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if ttl <= 0 || ttl > 10*time.Second {
		t.Fatalf("unexpected TTL %v", ttl)
	}

	allowed, err := middleware.AllowRateLimit(ctx, rdb, name, key, 2, 10*time.Second, true)
	if err != nil {
		t.Fatalf("third AllowRateLimit: %v", err)
	}
	if allowed {
		t.Fatalf("third request should be rate limited")
	}

	ttlAfter, err := rdb.PTTL(ctx, redisKey).Result()
	if err != nil {
		t.Fatalf("PTTL after limit: %v", err)
	}
	if ttlAfter <= 0 {
		t.Fatalf("rate-limit key lost its TTL: %v", ttlAfter)
	}
}

func TestRedisRateLimitRepairsLegacyKeyWithoutTTL(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not configured")
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx := context.Background()
	name := "integration_legacy_no_ttl"
	key := "legacy-key"
	redisKey := "rl:" + name + ":" + key
	if err := rdb.Set(ctx, redisKey, 7, 0).Err(); err != nil {
		t.Fatalf("seed legacy key: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Del(context.Background(), redisKey).Err() })

	allowed, err := middleware.AllowRateLimit(ctx, rdb, name, key, 20, 10*time.Second, true)
	if err != nil {
		t.Fatalf("AllowRateLimit: %v", err)
	}
	if !allowed {
		t.Fatal("repaired legacy key should remain below the configured limit")
	}

	ttl, err := rdb.PTTL(ctx, redisKey).Result()
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if ttl <= 0 || ttl > 10*time.Second {
		t.Fatalf("legacy key was not repaired with a bounded TTL: %v", ttl)
	}
}
