//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/redis/go-redis/v9"
)

// Run this test in two distinct processes, with the same persistent Garnet
// data directory and an abruptly terminated server between phases.
// A positive Redis PING after recovery is not sufficient.
func TestLocalGarnetAOFRecoverSecurityState(t *testing.T) {
	phase := os.Getenv("TEST_GARNET_DURABILITY_STAGE")
	id := os.Getenv("TEST_GARNET_DURABILITY_ID")
	addr := os.Getenv("TEST_GARNET_ADDR")
	if phase == "" && id == "" && addr == "" {
		t.Skip("Garnet AOF restart verification requires an explicit disposable Windows CI environment")
	}
	if (phase != "write" && phase != "verify") || id == "" || addr == "" {
		t.Fatal("TEST_GARNET_DURABILITY_STAGE, TEST_GARNET_DURABILITY_ID and TEST_GARNET_ADDR required together")
	}

	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: 0})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Garnet connection unavailable: %v", err)
	}
	store := authinfra.NewRefreshTokenStore(client)
	activeID := fmt.Sprintf("esterium:aof:%s:active", id)
	usedID := fmt.Sprintf("esterium:aof:%s:used", id)
	userID := "test-user-" + id
	throttleKey := "test-throttle-" + id
	const throttleLimit = 3

	switch phase {
	case "write":
		if err := store.Save(ctx, activeID, userID, 300); err != nil {
			t.Fatalf("persist unconsumed refresh token: %v", err)
		}
		if err := store.Save(ctx, usedID, userID, 300); err != nil {
			t.Fatalf("persist consumed refresh token: %v", err)
		}
		ok, err := store.Consume(ctx, usedID, userID)
		if err != nil || !ok {
			t.Fatalf("first use of consumed refresh token failed: ok=%v err=%v", ok, err)
		}
		for i := 0; i < throttleLimit; i++ {
			allowed, err := middleware.AllowRateLimit(ctx, client, "aof_restore_contract",
				throttleKey, throttleLimit, 5*time.Minute, true)
			if err != nil || !allowed {
				t.Fatalf("rate limiter did not persist attempt %d: allowed=%v err=%v", i, allowed, err)
			}
		}
		// AOF acknowledgment must happen before this test process exits.
	case "verify":
		if ok, err := store.Consume(ctx, usedID, userID); err != nil || ok {
			t.Fatalf("already-consumed token revived after recovery: ok=%v err=%v", ok, err)
		}
		if ok, err := store.Consume(ctx, activeID, userID); err != nil || !ok {
			t.Fatalf("unused token lost after recovery: ok=%v err=%v", ok, err)
		}
		if ok, err := store.Consume(ctx, activeID, userID); err != nil || ok {
			t.Fatalf("recovered token accepted a second use: ok=%v err=%v", ok, err)
		}
		allowed, err := middleware.AllowRateLimit(ctx, client, "aof_restore_contract",
			throttleKey, throttleLimit, 5*time.Minute, true)
		if err != nil || allowed {
			t.Fatalf("throttling state lost after recovery: allowed=%v err=%v", allowed, err)
		}
	}
}
