package infrastructure

import (
	"context"
	"fmt"
	"strings"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductsRepo struct {
	db *pgxpool.Pool
}

func NewProductsRepo(dbpool *pgxpool.Pool) *ProductsRepo {
	return &ProductsRepo{db: dbpool}
}

func (r *ProductsRepo) List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error) {
	q := strings.TrimSpace(query)
	where := "WHERE p.tenant_id=$1"
	args := []any{tenantID}
	if q != "" {
		where += " AND (p.name ILIKE $2 OR p.sku ILIKE $2 OR p.barcode ILIKE $2)"
		args = append(args, "%"+q+"%")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	countSQL := "SELECT count(*) FROM products p " + where
	var total int
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	listSQL := fmt.Sprintf(`
		SELECT p.id::text, p.category_id::text, p.sku, p.barcode, p.name, p.description, p.unit,
		       p.cost_price::float8, p.price_cash::float8, p.promo_price::float8, p.min_stock::float8, p.active,
		       COALESCE(b.qty_on_hand, 0)::float8
		FROM products p
		LEFT JOIN inventory_balances b ON b.product_id = p.id AND b.tenant_id = p.tenant_id
		%s
		ORDER BY p.name
		LIMIT $%d OFFSET $%d
	`, where, limitIdx, offsetIdx)

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []inv.Product
	for rows.Next() {
		var p inv.Product
		var categoryID *string
		var promo *float64
		var barcode *string
		var desc *string
		if err := rows.Scan(&p.ID, &categoryID, &p.SKU, &barcode, &p.Name, &desc, &p.Unit, &p.CostPrice, &p.PriceCash, &promo, &p.MinStock, &p.Active, &p.QtyOnHand); err != nil {
			return nil, 0, err
		}
		p.CategoryID = categoryID
		p.PromoPrice = promo
		p.Barcode = barcode
		p.Description = desc
		items = append(items, p)
	}
	return items, total, rows.Err()
}

func (r *ProductsRepo) Get(ctx context.Context, tenantID string, id string) (inv.Product, error) {
	var p inv.Product
	var categoryID *string
	var barcode *string
	var desc *string
	var promo *float64
	err := r.db.QueryRow(ctx, `
		SELECT p.id::text, p.category_id::text, p.sku, p.barcode, p.name, p.description, p.unit,
		       p.cost_price::float8, p.price_cash::float8, p.promo_price::float8, p.min_stock::float8, p.active,
		       COALESCE(b.qty_on_hand, 0)::float8
		FROM products p
		LEFT JOIN inventory_balances b ON b.product_id = p.id AND b.tenant_id = p.tenant_id
		WHERE p.tenant_id=$1 AND p.id=$2
	`, tenantID, id).Scan(&p.ID, &categoryID, &p.SKU, &barcode, &p.Name, &desc, &p.Unit, &p.CostPrice, &p.PriceCash, &promo, &p.MinStock, &p.Active, &p.QtyOnHand)
	p.CategoryID = categoryID
	p.Barcode = barcode
	p.Description = desc
	p.PromoPrice = promo
	return p, err
}

func (r *ProductsRepo) Create(ctx context.Context, tx db.DBTX, tenantID string, p inv.Product) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO products(tenant_id, category_id, sku, barcode, name, description, unit, cost_price, price_cash, promo_price, min_stock, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text
	`, tenantID, p.CategoryID, p.SKU, p.Barcode, p.Name, p.Description, p.Unit, p.CostPrice, p.PriceCash, p.PromoPrice, p.MinStock, p.Active).
		Scan(&id)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO inventory_balances(tenant_id, product_id, qty_on_hand) VALUES ($1,$2,0) ON CONFLICT (product_id) DO NOTHING`, tenantID, id)
	return id, err
}

func (r *ProductsRepo) Update(ctx context.Context, tx db.DBTX, tenantID string, id string, p inv.Product) error {
	_, err := tx.Exec(ctx, `
		UPDATE products
		SET category_id=$2, sku=$3, barcode=$4, name=$5, description=$6, unit=$7,
		    cost_price=$8, price_cash=$9, promo_price=$10, min_stock=$11, active=$12, updated_at=now()
		WHERE tenant_id=$1 AND id=$13
	`, tenantID, p.CategoryID, p.SKU, p.Barcode, p.Name, p.Description, p.Unit, p.CostPrice, p.PriceCash, p.PromoPrice, p.MinStock, p.Active, id)
	return err
}

func (r *ProductsRepo) GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, category_id::text, sku, barcode, name, description, unit,
		       cost_price::float8, price_cash::float8, promo_price::float8, min_stock::float8, active
		FROM products
		WHERE tenant_id=$1 AND id = ANY($2::uuid[])
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := map[string]inv.Product{}
	for rows.Next() {
		var p inv.Product
		var categoryID *string
		var barcode *string
		var desc *string
		var promo *float64
		if err := rows.Scan(&p.ID, &categoryID, &p.SKU, &barcode, &p.Name, &desc, &p.Unit, &p.CostPrice, &p.PriceCash, &promo, &p.MinStock, &p.Active); err != nil {
			return nil, err
		}
		p.CategoryID = categoryID
		p.Barcode = barcode
		p.Description = desc
		p.PromoPrice = promo
		m[p.ID] = p
	}
	return m, rows.Err()
}
