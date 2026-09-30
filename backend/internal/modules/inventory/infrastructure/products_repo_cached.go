package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/redis/go-redis/v9"
)

// CachedProductsRepo is a thin Redis cache in front of ProductsRepo.
// It is intentionally conservative: it caches only the common list shape
// (query="", offset=0, limit<=200) and individual Get(id).
//
// Invalidation is exposed via methods that the service can call AFTER commit.
// (We avoid invalidating inside Create/Update because those operations run inside a DB tx.)
type CachedProductsRepo struct {
	base *ProductsRepo
	rdb  *redis.Client
	ttl  time.Duration
}

func NewCachedProductsRepo(base *ProductsRepo, rdb *redis.Client) *CachedProductsRepo {
	return &CachedProductsRepo{base: base, rdb: rdb, ttl: 5 * time.Minute}
}

func (r *CachedProductsRepo) cacheEnabled() bool {
	return r != nil && r.base != nil && r.rdb != nil && r.ttl > 0
}

func productCacheKey(tenantID, id string) string {
	return "cache:products:tenant:" + strings.TrimSpace(tenantID) + ":get:" + strings.TrimSpace(id)
}

func productsListVerKey(tenantID string) string {
	return "cache:products:tenant:" + strings.TrimSpace(tenantID) + ":list:ver"
}

func (r *CachedProductsRepo) getProductsListVersion(ctx context.Context, tenantID string) int64 {
	if !r.cacheEnabled() {
		return 0
	}
	ver, err := r.rdb.Get(ctx, productsListVerKey(tenantID)).Int64()
	if err == nil {
		if ver <= 0 {
			return 1
		}
		return ver
	}
	if err == redis.Nil {
		_ = r.rdb.Set(ctx, productsListVerKey(tenantID), 1, 0).Err()
		return 1
	}
	// If Redis is flaky, just bypass caching.
	return 1
}

func productsListCacheKey(tenantID string, ver int64, query string, limit, offset int) string {
	// We cache only query="" currently, but keep the signature flexible.
	q := strings.TrimSpace(query)
	return fmt.Sprintf("cache:products:tenant:%s:list:v%d:q=%s:l=%d:o=%d", strings.TrimSpace(tenantID), ver, q, limit, offset)
}

type cachedProductsList struct {
	Items []inv.Product `json:"items"`
	Total int           `json:"total"`
}

func normalizeLimitOffset(limit, offset int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (r *CachedProductsRepo) List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error) {
	if !r.cacheEnabled() {
		return r.base.List(ctx, tenantID, query, limit, offset)
	}
	q := strings.TrimSpace(query)
	limit, offset = normalizeLimitOffset(limit, offset)

	// Only cache the most frequent shape used by the UI (PDV/Estoque): full list, first page.
	if q != "" || offset != 0 || limit > 200 {
		return r.base.List(ctx, tenantID, q, limit, offset)
	}

	ver := r.getProductsListVersion(ctx, tenantID)
	key := productsListCacheKey(tenantID, ver, q, limit, offset)
	if b, err := r.rdb.Get(ctx, key).Bytes(); err == nil {
		var v cachedProductsList
		if json.Unmarshal(b, &v) == nil {
			return v.Items, v.Total, nil
		}
	} else if err != redis.Nil {
		// ignore and fall back to DB
	}

	items, total, err := r.base.List(ctx, tenantID, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	if b, merr := json.Marshal(cachedProductsList{Items: items, Total: total}); merr == nil {
		_ = r.rdb.Set(ctx, key, b, r.ttl).Err()
	}
	return items, total, nil
}

func (r *CachedProductsRepo) Get(ctx context.Context, tenantID string, id string) (inv.Product, error) {
	if !r.cacheEnabled() {
		return r.base.Get(ctx, tenantID, id)
	}
	key := productCacheKey(tenantID, id)
	if b, err := r.rdb.Get(ctx, key).Bytes(); err == nil {
		var p inv.Product
		if json.Unmarshal(b, &p) == nil {
			return p, nil
		}
	} else if err != redis.Nil {
		// ignore and fall back to DB
	}

	p, err := r.base.Get(ctx, tenantID, id)
	if err != nil {
		return inv.Product{}, err
	}
	if b, merr := json.Marshal(p); merr == nil {
		_ = r.rdb.Set(ctx, key, b, r.ttl).Err()
	}
	return p, nil
}

func (r *CachedProductsRepo) GetByBarcode(ctx context.Context, tenantID string, barcode string) (inv.Product, error) {
	return r.base.GetByBarcode(ctx, tenantID, barcode)
}

func (r *CachedProductsRepo) Create(ctx context.Context, tx db.DBTX, tenantID string, p inv.Product) (string, error) {
	return r.base.Create(ctx, tx, tenantID, p)
}

func (r *CachedProductsRepo) Update(ctx context.Context, tx db.DBTX, tenantID string, id string, p inv.Product, preserveCost bool) error {
	return r.base.Update(ctx, tx, tenantID, id, p, preserveCost)
}

func (r *CachedProductsRepo) GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error) {
	return r.base.GetManyByIDs(ctx, tx, tenantID, ids)
}

// InvalidateProduct invalidates the per-product cache key.
// Call it after a successful commit.
func (r *CachedProductsRepo) InvalidateProduct(ctx context.Context, tenantID string, id string) error {
	if !r.cacheEnabled() {
		return nil
	}
	return r.rdb.Del(ctx, productCacheKey(tenantID, id)).Err()
}

// BumpProductsListVersion invalidates list cache by versioning.
// Call it after a successful commit that mutates products.
func (r *CachedProductsRepo) BumpProductsListVersion(ctx context.Context, tenantID string) error {
	if !r.cacheEnabled() {
		return nil
	}
	return r.rdb.Incr(ctx, productsListVerKey(tenantID)).Err()
}
