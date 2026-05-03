package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimitKeyFunc func(*http.Request) string

var sharedFallbackLimiter = newLocalRateLimiter()

func AllowRateLimit(ctx context.Context, rdb *redis.Client, name, key string, limit int, window time.Duration) bool {
	if limit <= 0 || window <= 0 {
		return true
	}
	allowed, err := allowRequest(ctx, rdb, sharedFallbackLimiter, name, key, limit, window)
	if err != nil {
		return sharedFallbackLimiter.allow(name+":"+key, limit, window)
	}
	return allowed
}

func RateLimit(rdb *redis.Client, name string, limit int, window time.Duration, keyFn RateLimitKeyFunc) func(http.Handler) http.Handler {
	if limit <= 0 || window <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	local := newLocalRateLimiter()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyPart := strings.TrimSpace(keyFn(r))
			if keyPart == "" {
				keyPart = "anonymous"
			}
			allowed, err := allowRequest(r.Context(), rdb, local, name, keyPart, limit, window)
			if err != nil {
				allowed = local.allow(name+":"+keyPart, limit, window)
			}
			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", window.Seconds()))
				writeMiddlewareError(w, r, http.StatusTooManyRequests, "rate_limit", "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func HashRateLimitIdentifier(identifier string) string {
	normalized := strings.ToLower(strings.TrimSpace(identifier))
	if normalized == "" {
		normalized = "blank"
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func RateLimitByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func RateLimitByTenantUserOrIP(r *http.Request) string {
	if au, ok := GetAuthUser(r.Context()); ok {
		return au.TenantID + ":" + au.UserID
	}
	return RateLimitByIP(r)
}

func allowRequest(ctx context.Context, rdb *redis.Client, local *localRateLimiter, name, key string, limit int, window time.Duration) (bool, error) {
	if rdb == nil {
		return local.allow(name+":"+key, limit, window), nil
	}
	redisKey := "rl:" + name + ":" + key
	redisCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	count, err := rdb.Incr(redisCtx, redisKey).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := rdb.Expire(redisCtx, redisKey, window).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(limit), nil
}

type localRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]localRateBucket
}

type localRateBucket struct {
	count   int
	expires time.Time
}

func newLocalRateLimiter() *localRateLimiter {
	return &localRateLimiter{buckets: make(map[string]localRateBucket)}
}

func (l *localRateLimiter) allow(key string, limit int, window time.Duration) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.buckets[key]
	if bucket.expires.IsZero() || now.After(bucket.expires) {
		l.buckets[key] = localRateBucket{count: 1, expires: now.Add(window)}
		return true
	}
	bucket.count++
	l.buckets[key] = bucket
	return bucket.count <= limit
}
