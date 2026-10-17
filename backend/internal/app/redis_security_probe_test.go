package app

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestRedisSecurityProbeRejectsMissingBackend(t *testing.T) {
	if err := verifyRedisSecurityPrimitives(context.Background(), nil); err == nil {
		t.Fatal("security probe accepted missing backend")
	}
}

func TestRedisSecurityProbeAgainstLiveRedis(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR not set; live Redis integration is a separate CI gate")
	}
	client := redis.NewClient(&redis.Options{Addr: address, MaxRetries: 0})
	defer client.Close()
	if err := verifyRedisSecurityPrimitives(context.Background(), client); err != nil {
		t.Fatalf("Redis security primitives must be atomic: %v", err)
	}
}
