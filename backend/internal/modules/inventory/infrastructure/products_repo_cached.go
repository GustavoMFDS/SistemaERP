package infrastructure

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/redis/go-redis/v9"
)

// CachedProductsRepo is a thin Redis cache in front of ProductsRepo.
// It is intentionally conservative: operational list queries always hit PostgreSQL
// because they include live price/stock; only individual Get(id) reads are cached.
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

func (r *CachedProductsRepo) List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error) {
	// Operational catalog lists include live price and stock. Always read them from
	// PostgreSQL so a transient Redis invalidation failure cannot resurrect stale
	// inventory or pricing in the PDV.
	return r.base.List(ctx, tenantID, query, limit, offset)
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

func (r *CachedProductsRepo) GetManyBySKUs(ctx context.Context, tx db.DBTX, tenantID string, skus []string) (map[string]inv.Product, error) {
	return r.base.GetManyBySKUs(ctx, tx, tenantID, skus)
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

// BumpProductsListVersion remains for service compatibility. Operational list
// queries are intentionally uncached, so there is no list version to advance.
func (r *CachedProductsRepo) BumpProductsListVersion(context.Context, string) error {
	return nil
}
