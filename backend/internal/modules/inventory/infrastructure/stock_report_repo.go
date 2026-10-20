package infrastructure

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

// StockReport includes all active/inactive catalog items, bounded by one
// extra row so downloads over the server limit are rejected, not truncated.
func (r *InventoryRepo) StockReport(ctx context.Context, tenantID string, limit int) ([]inv.StockReportRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.sku, p.name, p.unit,
			COALESCE(b.qty_on_hand, 0)::text, p.min_stock::text,
			p.active, COALESCE(b.qty_on_hand, 0) <= p.min_stock
		FROM products p
		LEFT JOIN inventory_balances b
		  ON b.tenant_id=p.tenant_id AND b.product_id=p.id
		WHERE p.tenant_id=$1
		ORDER BY p.sku, p.id
		LIMIT $2
	`, tenantID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]inv.StockReportRow, 0)
	for rows.Next() {
		var item inv.StockReportRow
		if err := rows.Scan(&item.SKU, &item.Name, &item.Unit,
			&item.Quantity, &item.Minimum, &item.Active, &item.BelowLimit); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
