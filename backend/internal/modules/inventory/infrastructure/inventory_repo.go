package infrastructure

import (
	"context"
	"fmt"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InventoryRepo struct {
	db *pgxpool.Pool
}

func NewInventoryRepo(dbpool *pgxpool.Pool) *InventoryRepo {
	return &InventoryRepo{db: dbpool}
}

// Opening stock requires an immutable batch key. Advisory locks serialize
// retries before reading the committed batch record.
func (r *InventoryRepo) LockOpeningStockKey(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`,
		tenantID+":opening-stock:"+key)
	return err
}

func (r *InventoryRepo) GetOpeningStockBatch(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) (batchID, requestHash string, itemCount int, found bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id::text, request_hash, item_count
		FROM opening_stock_batches
		WHERE tenant_id=$1 AND idem_key=$2
	`, tenantID, key).Scan(&batchID, &requestHash, &itemCount)
	if err == pgx.ErrNoRows {
		return "", "", 0, false, nil
	}
	if err != nil {
		return "", "", 0, false, err
	}
	return batchID, requestHash, itemCount, true, nil
}

// Read-only reconciliation is scoped to the authenticated company's tenant ID.
// It reveals no CSV row data or request digest.
func (r *InventoryRepo) LookupOpeningStockBatch(
	ctx context.Context, tenantID, key string,
) (batchID string, itemCount int, found bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT id::text, item_count
		FROM opening_stock_batches
		WHERE tenant_id=$1 AND idem_key=$2
	`, tenantID, key).Scan(&batchID, &itemCount)
	if err == pgx.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return batchID, itemCount, true, nil
}

func (r *InventoryRepo) CreateOpeningStockBatch(
	ctx context.Context, tx db.DBTX, tenantID, actorID, key, requestHash string, itemCount int,
) (string, error) {
	var batchID string
	err := tx.QueryRow(ctx, `
		INSERT INTO opening_stock_batches(
			tenant_id, idem_key, request_hash, item_count, created_by_user_id
		) VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text
	`, tenantID, key, requestHash, itemCount, actorID).Scan(&batchID)
	return batchID, err
}

// All balance rows must already be locked by the caller. A movement made by
// another transaction cannot commit past those locks between this check and
// the opening writes.
func (r *InventoryRepo) HasAnyStockMovements(
	ctx context.Context, tx db.DBTX, tenantID string, productIDs []string,
) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM inventory_movements
			WHERE tenant_id=$1 AND product_id=ANY($2::uuid[])
		)
	`, tenantID, productIDs).Scan(&found)
	return found, err
}

func (r *InventoryRepo) EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID string, productID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO inventory_balances(tenant_id, product_id, qty_on_hand) VALUES ($1,$2,0) ON CONFLICT (product_id) DO NOTHING`, tenantID, productID)
	return err
}

// EnsureBalanceRows inserts missing inventory_balance rows for a batch of products.
// This reduces N+1 queries during sale finalization.
func (r *InventoryRepo) EnsureBalanceRows(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) error {
	if len(productIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO inventory_balances(tenant_id, product_id, qty_on_hand)
		SELECT $1::uuid, unnest($2::uuid[]), 0
		ON CONFLICT (product_id) DO NOTHING
	`, tenantID, productIDs)
	return err
}

func (r *InventoryRepo) GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productID string) (inv.InventoryBalance, error) {
	var b inv.InventoryBalance
	var qty string
	err := tx.QueryRow(ctx, `SELECT product_id::text, qty_on_hand::text FROM inventory_balances WHERE tenant_id=$1 AND product_id=$2 FOR UPDATE`, tenantID, productID).
		Scan(&b.ProductID, &qty)
	if err != nil {
		return b, err
	}
	b.QtyOnHand, err = platform.ParseQuantity(qty)
	return b, err
}

// GetBalancesForUpdate locks and returns balances for all provided product IDs.
// productIDs should be unique and preferably sorted for stable locking order.
func (r *InventoryRepo) GetBalancesForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) (map[string]inv.InventoryBalance, error) {
	if len(productIDs) == 0 {
		return map[string]inv.InventoryBalance{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT product_id::text, qty_on_hand::text
		FROM inventory_balances
		WHERE tenant_id=$1 AND product_id = ANY($2::uuid[])
		ORDER BY product_id
		FOR UPDATE
	`, tenantID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]inv.InventoryBalance, len(productIDs))
	for rows.Next() {
		var b inv.InventoryBalance
		var qty string
		if err := rows.Scan(&b.ProductID, &qty); err != nil {
			return nil, err
		}
		q, err := platform.ParseQuantity(qty)
		if err != nil {
			return nil, err
		}
		b.QtyOnHand = q
		out[b.ProductID] = b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InventoryRepo) UpdateBalance(ctx context.Context, tx db.DBTX, tenantID string, productID string, qty platform.Quantity) error {
	_, err := tx.Exec(ctx, `UPDATE inventory_balances SET qty_on_hand=$3, updated_at=now() WHERE tenant_id=$1 AND product_id=$2`, tenantID, productID, qty.DBString())
	return err
}

func (r *InventoryRepo) InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, m inv.InventoryMovement) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO inventory_movements(tenant_id, product_id, movement_type, delta, qty_before, qty_after, reason, reference_type, reference_id, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, tenantID, m.ProductID, m.MovementType, m.Delta.DBString(), m.QtyBefore.DBString(), m.QtyAfter.DBString(), m.Reason, m.ReferenceType, m.ReferenceID, m.ActorUserID)
	return err
}

// LowStockCount counts the full tenant catalog, independently of the paged UI list.
func (r *InventoryRepo) LowStockCount(ctx context.Context, tenantID string) (int, error) {
	var total int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM products p
		LEFT JOIN inventory_balances b
		  ON b.product_id=p.id AND b.tenant_id=p.tenant_id
		WHERE p.tenant_id=$1 AND p.active=true
		  AND COALESCE(b.qty_on_hand, 0) <= p.min_stock
	`, tenantID).Scan(&total)
	return total, err
}

func (r *InventoryRepo) LowStock(ctx context.Context, tenantID string, limit int) ([]inv.Product, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.category_id::text, p.sku, p.barcode, p.name, p.description, p.unit,
		       p.cost_price::text, p.price_cash::text, p.promo_price::text, p.min_stock::text, p.active,
		       COALESCE(b.qty_on_hand, 0)::text
		FROM products p
		LEFT JOIN inventory_balances b ON b.product_id=p.id AND b.tenant_id=p.tenant_id
		WHERE p.tenant_id=$1 AND p.active=true AND COALESCE(b.qty_on_hand,0) <= p.min_stock
		ORDER BY (p.min_stock - COALESCE(b.qty_on_hand,0)) DESC
		LIMIT $2
	`, tenantID, limit)
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
		var costPrice, priceCash, minStock, qtyOnHand string
		var promo *string
		if err := rows.Scan(&p.ID, &categoryID, &p.SKU, &barcode, &p.Name, &desc, &p.Unit, &costPrice, &priceCash, &promo, &minStock, &p.Active, &qtyOnHand); err != nil {
			return nil, err
		}
		if err := assignProductNumbers(&p, costPrice, priceCash, promo, minStock, qtyOnHand); err != nil {
			return nil, err
		}
		p.CategoryID = categoryID
		p.Barcode = barcode
		p.Description = desc
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *InventoryRepo) ListMovements(ctx context.Context, tenantID string, productID string, limit, offset int) ([]inv.InventoryMovement, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE m.tenant_id=$1"
	args := []any{tenantID}
	if productID != "" {
		where += " AND m.product_id=$2"
		args = append(args, productID)
	}

	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM inventory_movements m "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, `
		SELECT m.id::text, m.product_id::text, p.sku, p.name, m.movement_type,
		       m.delta::text, m.qty_before::text, m.qty_after::text,
		       m.reason, m.reference_type, m.reference_id::text,
		       m.actor_user_id::text, m.created_at::text
		FROM inventory_movements m
		JOIN products p ON p.id=m.product_id AND p.tenant_id=m.tenant_id
		`+where+`
		ORDER BY m.created_at DESC, m.id DESC
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
		var delta, qtyBefore, qtyAfter string
		if err := rows.Scan(&m.ID, &m.ProductID, &m.ProductSKU, &m.ProductName, &m.MovementType, &delta, &qtyBefore, &qtyAfter, &m.Reason, &m.ReferenceType, &refID, &actor, &m.CreatedAt); err != nil {
			return nil, 0, err
		}
		var err error
		if m.Delta, err = platform.ParseQuantity(delta); err != nil {
			return nil, 0, err
		}
		if m.QtyBefore, err = platform.ParseQuantity(qtyBefore); err != nil {
			return nil, 0, err
		}
		if m.QtyAfter, err = platform.ParseQuantity(qtyAfter); err != nil {
			return nil, 0, err
		}
		m.ReferenceID = refID
		m.ActorUserID = actor
		items = append(items, m)
	}
	return items, total, rows.Err()
}
