package infrastructure

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

// ListUnifiedImportHistory applies tenant and each permission to the SQL
// branches before UNION ALL. Pagination is global across both kinds; paging
// each source separately would omit or reorder mixed receipts.
func (r *ProductsRepo) ListUnifiedImportHistory(
	ctx context.Context, tenantID string, allowProducts, allowOpeningStock bool,
	limit, offset int, from, to string,
) ([]inv.UnifiedImportReceipt, bool, error) {
	var fromArg, toArg any
	if from != "" {
		fromArg = from
	}
	if to != "" {
		toArg = to
	}
	rows, err := r.db.Query(ctx, `
		WITH eligible AS (
			SELECT p.id::text AS batch_id, 'products'::text AS kind,
				p.item_count, u.name AS actor_name, p.created_at
			FROM product_import_batches p
			JOIN users u ON u.id = p.created_by_user_id
			WHERE p.tenant_id = $1 AND $4::boolean
			  AND ($6::date IS NULL OR p.created_at >= ($6::date::timestamp AT TIME ZONE 'America/Sao_Paulo'))
			  AND ($7::date IS NULL OR p.created_at < (($7::date + 1)::timestamp AT TIME ZONE 'America/Sao_Paulo'))
			UNION ALL
			SELECT s.id::text AS batch_id, 'opening-stock'::text AS kind,
				s.item_count, u.name AS actor_name, s.created_at
			FROM opening_stock_batches s
			JOIN users u ON u.id = s.created_by_user_id
			WHERE s.tenant_id = $1 AND $5::boolean
			  AND ($6::date IS NULL OR s.created_at >= ($6::date::timestamp AT TIME ZONE 'America/Sao_Paulo'))
			  AND ($7::date IS NULL OR s.created_at < (($7::date + 1)::timestamp AT TIME ZONE 'America/Sao_Paulo'))
		)
		SELECT batch_id, kind, item_count, actor_name, created_at
		FROM eligible
		ORDER BY created_at DESC, kind ASC, batch_id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit+1, offset, allowProducts, allowOpeningStock, fromArg, toArg)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]inv.UnifiedImportReceipt, 0, limit)
	for rows.Next() {
		var item inv.UnifiedImportReceipt
		if err := rows.Scan(&item.BatchID, &item.Kind, &item.ItemCount, &item.ActorName, &item.CreatedAt); err != nil {
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

func (r *CachedProductsRepo) ListUnifiedImportHistory(
	ctx context.Context, tenantID string, allowProducts, allowOpeningStock bool,
	limit, offset int, from, to string,
) ([]inv.UnifiedImportReceipt, bool, error) {
	// Never cache data scoped to a combination of permissions and tenant.
	return r.base.ListUnifiedImportHistory(ctx, tenantID, allowProducts, allowOpeningStock, limit, offset, from, to)
}
