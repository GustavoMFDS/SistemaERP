// Package cache implements a Redis-backed read-through cache for frequently
// accessed, slow-changing data (product catalog, categories).
//
// Cache strategy:
//   - Read-through: on miss, load from DB and populate cache transparently.
//   - Write-invalidate: on product update/create, delete the cached entry
//     so the next read fetches fresh data from DB.
//   - TTL: 5 minutes default — product prices change infrequently.
//     Operators who update a price see it reflected in < 5 min automatically.
//   - Batch operations (GetManyByIDs) fetch uncached IDs in a single DB query.
//
// Key schema:
//   product:{tenantID}:{productID}   → JSON-encoded Product
//   products:tenant:{tenantID}:list  → JSON-encoded []Product (paginated list)
//
// The list key is deleted on any product mutation (conservative invalidation).
// Individual product keys survive until TTL unless explicitly invalidated.

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/redis/go-redis/v9"
)

const (
	DefaultProductTTL = 5 * time.Minute
	DefaultListTTL    = 2 * time.Minute
)

// ProductCache wraps a Redis client and provides typed product caching.
type ProductCache struct {
	rdb        *redis.Client
	productTTL time.Duration
	listTTL    time.Duration
}

// NewProductCache creates a ProductCache with the provided Redis client.
func NewProductCache(rdb *redis.Client, opts ...CacheOption) *ProductCache {
	c := &ProductCache{
		rdb:        rdb,
		productTTL: DefaultProductTTL,
		listTTL:    DefaultListTTL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// CacheOption configures ProductCache.
type CacheOption func(*ProductCache)

func WithProductTTL(d time.Duration) CacheOption { return func(c *ProductCache) { c.productTTL = d } }
func WithListTTL(d time.Duration) CacheOption    { return func(c *ProductCache) { c.listTTL = d } }

// ── Key helpers ───────────────────────────────────────────────────────────────

func productKey(tenantID, productID string) string {
	return fmt.Sprintf("product:%s:%s", tenantID, productID)
}

func listKey(tenantID string) string {
	return fmt.Sprintf("products:tenant:%s:list", tenantID)
}

// ── Get ───────────────────────────────────────────────────────────────────────

// GetByID returns a cached product, or (zero, false, nil) on miss.
// Returns a non-nil error only for Redis connection problems.
func (c *ProductCache) GetByID(ctx context.Context, tenantID, productID string) (inv.Product, bool, error) {
	data, err := c.rdb.Get(ctx, productKey(tenantID, productID)).Bytes()
	if err == redis.Nil {
		return inv.Product{}, false, nil
	}
	if err != nil {
		return inv.Product{}, false, fmt.Errorf("cache get: %w", err)
	}
	var p inv.Product
	if err := json.Unmarshal(data, &p); err != nil {
		// Corrupt entry — treat as miss and let caller refresh.
		_ = c.rdb.Del(ctx, productKey(tenantID, productID))
		return inv.Product{}, false, nil
	}
	return p, true, nil
}

// GetManyByIDs returns cached products by ID.
// Returns hits (map) and misses (slice of IDs not found in cache).
// Uses MGET for a single round-trip regardless of slice size.
func (c *ProductCache) GetManyByIDs(ctx context.Context, tenantID string, ids []string) (hits map[string]inv.Product, misses []string, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = productKey(tenantID, id)
	}

	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		// On Redis error, treat all as misses (graceful degradation).
		return nil, ids, nil
	}

	hits = make(map[string]inv.Product, len(ids))
	for i, val := range vals {
		if val == nil {
			misses = append(misses, ids[i])
			continue
		}
		raw, ok := val.(string)
		if !ok {
			misses = append(misses, ids[i])
			continue
		}
		var p inv.Product
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			misses = append(misses, ids[i])
			continue
		}
		hits[ids[i]] = p
	}
	return hits, misses, nil
}

// ── Set ───────────────────────────────────────────────────────────────────────

// SetByID stores a product in cache. Errors are soft (caller should log but not fail).
func (c *ProductCache) SetByID(ctx context.Context, tenantID string, p inv.Product) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	return c.rdb.Set(ctx, productKey(tenantID, p.ID), data, c.productTTL).Err()
}

// SetMany stores multiple products in a single pipeline round-trip.
func (c *ProductCache) SetMany(ctx context.Context, tenantID string, products []inv.Product) error {
	if len(products) == 0 {
		return nil
	}
	pipe := c.rdb.Pipeline()
	for _, p := range products {
		data, err := json.Marshal(p)
		if err != nil {
			continue // skip corrupt product
		}
		pipe.Set(ctx, productKey(tenantID, p.ID), data, c.productTTL)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ── Invalidate ────────────────────────────────────────────────────────────────

// InvalidateProduct removes a single product from cache.
// Call after any product mutation (update price, deactivate, etc.).
func (c *ProductCache) InvalidateProduct(ctx context.Context, tenantID, productID string) error {
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, productKey(tenantID, productID))
	pipe.Del(ctx, listKey(tenantID)) // also bust the list
	_, err := pipe.Exec(ctx)
	return err
}

// InvalidateAll removes all cached products for a tenant.
// Use with caution — causes a full cache warm-up on next access.
func (c *ProductCache) InvalidateAll(ctx context.Context, tenantID string) error {
	iter := c.rdb.Scan(ctx, 0, fmt.Sprintf("product:%s:*", tenantID), 500).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	keys = append(keys, listKey(tenantID))
	if len(keys) == 0 {
		return nil
	}
	return c.rdb.Del(ctx, keys...).Err()
}

// ── Read-through helper ───────────────────────────────────────────────────────

// ProductLoader is any function that loads products from the DB by IDs.
type ProductLoader func(ctx context.Context, tenantID string, ids []string) (map[string]inv.Product, error)

// GetManyWithReadThrough fetches products using cache-first strategy.
// On partial miss, calls loader for the missing IDs and populates the cache.
//
// Usage in ProductsRepo.GetManyByIDs wrapper:
//
//	return cache.GetManyWithReadThrough(ctx, tenantID, ids, dbLoader)
func (c *ProductCache) GetManyWithReadThrough(
	ctx context.Context,
	tenantID string,
	ids []string,
	loader ProductLoader,
) (map[string]inv.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	hits, misses, err := c.GetManyByIDs(ctx, tenantID, ids)
	if err != nil {
		// Cache unavailable — fall through to DB.
		return loader(ctx, tenantID, ids)
	}

	if len(misses) == 0 {
		return hits, nil
	}

	// Load misses from DB.
	dbResults, err := loader(ctx, tenantID, misses)
	if err != nil {
		return nil, err
	}

	// Populate cache for future requests (fire-and-forget, non-blocking).
	go func() {
		freshCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, p := range dbResults {
			_ = c.SetByID(freshCtx, tenantID, p)
		}
	}()

	// Merge hits + fresh DB results.
	result := make(map[string]inv.Product, len(ids))
	for id, p := range hits {
		result[id] = p
	}
	for id, p := range dbResults {
		result[id] = p
	}
	return result, nil
}
