package infrastructure

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
)

// ProductRanking is strictly scoped to finalized sales in one company, with
// business-day boundaries in Sao Paulo and no cost/fiscal fields.
func (r *FinanceRepo) ProductRanking(
	ctx context.Context, tenantID, from, to string, limit int,
) ([]fin.ProductRankingRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.sku, p.name,
			COALESCE(SUM(si.qty),0)::text,
			COALESCE(SUM(si.subtotal),0)::text,
			COUNT(DISTINCT s.id)
		FROM sale_items si
		JOIN sales s ON s.id=si.sale_id AND s.tenant_id=si.tenant_id
		JOIN products p ON p.id=si.product_id AND p.tenant_id=s.tenant_id
		WHERE s.tenant_id=$1 AND s.status='finalized'
		  AND s.finalized_at >= ($2::date::timestamp AT TIME ZONE 'America/Sao_Paulo')
		  AND s.finalized_at < (($3::date + 1)::timestamp AT TIME ZONE 'America/Sao_Paulo')
		GROUP BY p.id,p.sku,p.name
		ORDER BY SUM(si.subtotal) DESC, p.sku, p.id
		LIMIT $4
	`, tenantID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]fin.ProductRankingRow, 0)
	for rows.Next() {
		var item fin.ProductRankingRow
		if err := rows.Scan(&item.ProductID, &item.SKU, &item.Name,
			&item.Quantity, &item.ItemTotal, &item.SalesCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
