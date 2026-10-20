package infrastructure

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MemoryRefreshTokenStore is intended exclusively for single-process
// development/demo environments when Redis is intentionally disabled.
// It is NOT persistent, distributed or suitable for staging/production.
// Token IDs are cryptographically random JWT JTI values, never raw JWTs.
type MemoryRefreshTokenStore struct {
	mu    sync.Mutex
	items map[string]memoryRefreshToken
}

type memoryRefreshToken struct {
	userID    string
	expiresAt time.Time
}

const maxDevRefreshTokens = 10000

func NewMemoryRefreshTokenStore() *MemoryRefreshTokenStore {
	return &MemoryRefreshTokenStore{items: make(map[string]memoryRefreshToken)}
}

func (s *MemoryRefreshTokenStore) Save(
	ctx context.Context, tokenID, userID string, ttlSeconds int64,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || tokenID == "" || userID == "" || ttlSeconds <= 0 {
		return errors.New("invalid development refresh token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// Do not retain indefinitely expired tokens when the demo runs all day.
	for id, entry := range s.items {
		if !entry.expiresAt.After(now) {
			delete(s.items, id)
		}
	}
	if _, exists := s.items[tokenID]; exists {
		return errors.New("duplicate development refresh token ID")
	}
	if len(s.items) >= maxDevRefreshTokens {
		return errors.New("development refresh token store capacity exceeded")
	}
	s.items[tokenID] = memoryRefreshToken{
		userID:    userID,
		expiresAt: now.Add(time.Duration(ttlSeconds) * time.Second),
	}
	return nil
}

func (s *MemoryRefreshTokenStore) Consume(
	ctx context.Context, tokenID, userID string,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil || tokenID == "" || userID == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.items[tokenID]
	if !found {
		return false, nil
	}
	// Regardless of matching principal or expiry, a presented JTI is never
	// left replayable after its first Consume attempt.
	delete(s.items, tokenID)
	return entry.userID == userID && entry.expiresAt.After(time.Now()), nil
}

func (s *MemoryRefreshTokenStore) Revoke(ctx context.Context, tokenID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	s.mu.Lock()
	delete(s.items, tokenID)
	s.mu.Unlock()
	return nil
}
