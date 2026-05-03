package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RefreshTokenStore struct {
	rdb *redis.Client
}

func NewRefreshTokenStore(rdb *redis.Client) *RefreshTokenStore {
	return &RefreshTokenStore{rdb: rdb}
}

func (s *RefreshTokenStore) key(tokenID string) string {
	return fmt.Sprintf("auth:refresh:%s", tokenID)
}

func (s *RefreshTokenStore) Save(ctx context.Context, tokenID string, userID string, ttlSeconds int64) error {
	if ttlSeconds <= 0 {
		return fmt.Errorf("invalid ttlSeconds")
	}
	return s.rdb.Set(ctx, s.key(tokenID), userID, timeSeconds(ttlSeconds)).Err()
}

// Consume atomically validates and revokes (deletes) a refresh token ID.
// It returns true only once per tokenID.
func (s *RefreshTokenStore) Consume(ctx context.Context, tokenID string, userID string) (bool, error) {
	const script = `
local k = KEYS[1]
local expected = ARGV[1]
local v = redis.call('GET', k)
if not v then
  return 0
end
if v ~= expected then
  return 0
end
redis.call('DEL', k)
return 1
`
	res, err := s.rdb.Eval(ctx, script, []string{s.key(tokenID)}, userID).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (s *RefreshTokenStore) Revoke(ctx context.Context, tokenID string) error {
	return s.rdb.Del(ctx, s.key(tokenID)).Err()
}

// timeSeconds is a tiny helper to avoid importing time in every file that needs redis TTL.
func timeSeconds(sec int64) time.Duration {
	return time.Duration(sec) * time.Second
}
