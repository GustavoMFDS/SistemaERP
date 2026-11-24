package app

import (
	"context"
	"errors"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/redis/go-redis/v9"
)

// TestGarnetNativeSecurityRecovery is driven twice by the disposable Windows
// AOF smoke, with a hard process stop between phases. It uses the production
// TLS configuration, token store and rate limiter rather than raw Redis alone.
func TestGarnetNativeSecurityRecovery(t *testing.T) {
	phase := os.Getenv("ESTERIUM_TEST_GARNET_RECOVERY_STAGE")
	id := os.Getenv("ESTERIUM_TEST_GARNET_RECOVERY_ID")
	password := os.Getenv("ESTERIUM_TEST_GARNET_RECOVERY_PASSWORD")
	cert := os.Getenv("ESTERIUM_TEST_GARNET_RECOVERY_CERT")
	if phase == "" && id == "" && password == "" && cert == "" {
		t.Skip("requires isolated native Garnet Windows CI")
	}
	if runtime.GOOS != "windows" || os.Getenv("GITHUB_ACTIONS") != "true" ||
		os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" ||
		(phase != "write" && phase != "verify") ||
		!regexp.MustCompile("^[0-9a-f]{32}$").MatchString(id) ||
		password != "ci-only-garnet-"+id || cert == "" {
		t.Fatal("native Garnet contract requires explicit disposable Windows CI fixture")
	}
	u := url.URL{Scheme: "rediss", Host: "localhost:64325", User: url.UserPassword("esteriumcache", password)}
	options, err := redis.ParseURL(u.String())
	if err != nil {
		t.Fatal("invalid synthetic TLS URL")
	}
	options.MaxRetries = -1
	if err := configureRedisTLS(config.Config{RedisURL: u.String(), RedisCACertFile: cert}, options); err != nil {
		t.Fatal("native Garnet TLS configuration failed")
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal("native Garnet authenticated TLS connection failed")
	}

	store := authinfra.NewRefreshTokenStore(client)
	user := "ci-user-" + id
	active, used, revoked := id+":active", id+":used", id+":revoked"
	expiring := id + ":expiring"
	const limit = 3
	const window = 5 * time.Minute
	throttle := func(want bool) {
		t.Helper()
		allowed, err := middleware.AllowRateLimit(ctx, client, "garnet_recovery_contract", id, limit, window, true)
		if err != nil || allowed != want {
			t.Fatalf("rate limit contract failed: allowed=%v want=%v", allowed, want)
		}
	}
	consume := func(token, owner string, want bool) {
		t.Helper()
		ok, err := store.Consume(ctx, token, owner)
		if err != nil || ok != want {
			t.Fatalf("refresh token contract failed: consumed=%v want=%v", ok, want)
		}
	}

	if phase == "write" {
		if err := verifyRedisSecurityPrimitives(ctx, client); err != nil {
			t.Fatal("production startup security probe failed")
		}
		for _, token := range []string{active, used, revoked} {
			if err := store.Save(ctx, token, user, 300); err != nil {
				t.Fatal("refresh token persistence failed")
			}
		}
		consume(active, "other-user", false)
		consume(used, user, true)
		if err := store.Revoke(ctx, revoked); err != nil {
			t.Fatal("refresh revocation failed")
		}
		if err := store.Save(ctx, expiring, user, 1); err != nil {
			t.Fatal("short-lived refresh token persistence failed")
		}
		time.Sleep(1500 * time.Millisecond)
		consume(expiring, user, false)
		for range limit {
			throttle(true)
		}
		throttle(false)
		return
	}

	consume(used, user, false)
	consume(revoked, user, false)
	consume(expiring, user, false)
	consume(active, user, true)
	consume(active, user, false)
	throttle(false)
	if allowed, err := middleware.AllowRateLimit(ctx, nil, "garnet_recovery_contract", id, limit, window, true); allowed || !errors.Is(err, middleware.ErrRateLimitUnavailable) {
		t.Fatal("security limiter accepted a missing backend")
	}
}
