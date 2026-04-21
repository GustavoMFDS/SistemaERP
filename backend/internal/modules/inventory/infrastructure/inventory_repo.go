package infrastructure

import (
	"context"
	"fmt"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InventoryRepo struct {
	db *pgxpool.Pool
}

func NewInventoryRepo(dbpool *pgxpool.Pool) *InventoryRepo {
	return &InventoryRepo{db: dbpool}
}

func (r *InventoryRepo) EnsureBalanceRow(ctx context.Context, tx db.DBTX, productID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO inventory_balances(product_id, qty_on_hand) VALUES ($1,0) ON CONFLICT DO NOTHING`, productID)
	return err
}

// EnsureBalanceRows inserts missing inventory_balance rows for a batch of products.
// This reduces N+1 queries during sale finalization.
func (r *InventoryRepo) EnsureBalanceRows(ctx context.Context, tx db.DBTX, productIDs []string) error {
	if len(productIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO inventory_balances(product_id, qty_on_hand)
		SELECT unnest($1::uuid[]), 0
		ON CONFLICT DO NOTHING
	`, productIDs)
	return err
}

func (r *InventoryRepo) GetBalanceForUpdate(ctx context.Context, tx db.DBTX, productID string) (inv.InventoryBalance, error) {
	var b inv.InventoryBalance
	err := tx.QueryRow(ctx, `SELECT product_id::text, qty_on_hand::float8 FROM inventory_balances WHERE product_id=$1 FOR UPDATE`, productID).
		Scan(&b.ProductID, &b.QtyOnHand)
	return b, err
}

// GetBalancesForUpdate locks and returns balances for all provided product IDs.
// productIDs should be unique and preferably sorted for stable locking order.
func (r *InventoryRepo) GetBalancesForUpdate(ctx context.Context, tx db.DBTX, productIDs []string) (map[string]inv.InventoryBalance, error) {
	if len(productIDs) == 0 {
		return map[string]inv.InventoryBalance{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT product_id::text, qty_on_hand::float8
		FROM inventory_balances
		WHERE product_id = ANY($1::uuid[])
		ORDER BY product_id
		FOR UPDATE
	`, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]inv.InventoryBalance, len(productIDs))
	for rows.Next() {
		var b inv.InventoryBalance
		if err := rows.Scan(&b.ProductID, &b.QtyOnHand); err != nil {
			return nil, err
		}
		out[b.ProductID] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InventoryRepo) UpdateBalance(ctx context.Context, tx db.DBTX, productID string, qty float64) error {
	_, err := tx.Exec(ctx, `UPDATE inventory_balances SET qty_on_hand=$2, updated_at=now() WHERE product_id=$1`, productID, qty)
	return err
}

func (r *InventoryRepo) InsertMovement(ctx context.Context, tx db.DBTX, m inv.InventoryMovement) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO inventory_movements(product_id, movement_type, delta, qty_before, qty_after, reason, reference_type, reference_id, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, m.ProductID, m.MovementType, m.Delta, m.QtyBefore, m.QtyAfter, m.Reason, m.ReferenceType, m.ReferenceID, m.ActorUserID)
	return err
}

func (r *InventoryRepo) LowStock(ctx context.Context, limit int) ([]inv.Product, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.category_id::text, p.sku, p.barcode, p.name, p.description, p.unit,
		       p.cost_price::float8, p.price_cash::float8, p.promo_price::float8, p.min_stock::float8, p.active,
		       COALESCE(b.qty_on_hand, 0)::float8
		FROM products p
		LEFT JOIN inventory_balances b ON b.product_id=p.id
		WHERE p.active=true AND COALESCE(b.qty_on_hand,0) <= p.min_stock
		ORDER BY (p.min_stock - COALESCE(b.qty_on_hand,0)) DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []inv.Product
	for rows.Next() {
		var p inv.Product
		var categoryID *string
		var barcode *string
		var desc *string
		var promo *float64
		if err := rows.Scan(&p.ID, &categoryID, &p.SKU, &barcode, &p.Name, &desc, &p.Unit, &p.CostPrice, &p.PriceCash, &promo, &p.MinStock, &p.Active, &p.QtyOnHand); err != nil {
			return nil, err
		}
		p.CategoryID = categoryID
		p.Barcode = barcode
		p.Description = desc
		p.PromoPrice = promo
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *InventoryRepo) ListMovements(ctx context.Context, productID string, limit, offset int) ([]inv.InventoryMovement, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE 1=1"
	args := []any{}
	if productID != "" {
		where += " AND product_id=$1"
		args = append(args, productID)
	}

	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM inventory_movements "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, `
		SELECT id::text, product_id::text, movement_type, delta::float8, qty_before::float8, qty_after::float8,
		       reason, reference_type, reference_id::text, actor_user_id::text, created_at::text
		FROM inventory_movements
		`+where+`
		ORDER BY created_at DESC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`
	`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []inv.InventoryMovement
	for rows.Next() {
		var m inv.InventoryMovement
		var refID *string
		var actor *string
		if err := rows.Scan(&m.ID, &m.ProductID, &m.MovementType, &m.Delta, &m.QtyBefore, &m.QtyAfter, &m.Reason, &m.ReferenceType, &refID, &actor, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		m.ReferenceID = refID
		m.ActorUserID = actor
		items = append(items, m)
	}
	return items, total, rows.Err()
}
