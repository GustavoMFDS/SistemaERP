package infrastructure

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Both table names are static internal constants; no untrusted SQL identifiers
// or tenant selectors are accepted from the HTTP request.
func listImportHistory(
	ctx context.Context, pool *pgxpool.Pool, table, tenantID string, limit, offset int,
) ([]inv.ImportBatchEntry, bool, error) {
	// limit + 1 lets the UI paginate without a costly global count.
	query := `
		SELECT b.id::text, b.item_count, u.name, b.created_at
		FROM ` + table + ` b
		JOIN users u ON u.id = b.created_by_user_id
		WHERE b.tenant_id = $1
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := pool.Query(ctx, query, tenantID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]inv.ImportBatchEntry, 0, limit)
	for rows.Next() {
		var item inv.ImportBatchEntry
		if err := rows.Scan(&item.BatchID, &item.ItemCount, &item.ActorName, &item.CreatedAt); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func (r *ProductsRepo) ListProductImportHistory(
	ctx context.Context, tenantID string, limit, offset int,
) ([]inv.ImportBatchEntry, bool, error) {
	return listImportHistory(ctx, r.db, "product_import_batches", tenantID, limit, offset)
}

func (r *CachedProductsRepo) ListProductImportHistory(
	ctx context.Context, tenantID string, limit, offset int,
) ([]inv.ImportBatchEntry, bool, error) {
	// Never cache audit history or reuse data across tenants.
	return r.base.ListProductImportHistory(ctx, tenantID, limit, offset)
}

func (r *InventoryRepo) ListOpeningStockHistory(
	ctx context.Context, tenantID string, limit, offset int,
) ([]inv.ImportBatchEntry, bool, error) {
	return listImportHistory(ctx, r.db, "opening_stock_batches", tenantID, limit, offset)
}
