package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/common"
	proc "github.com/example/sistemaemgo/internal/modules/procurement/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{db: pool}
}

func mapConstraintError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return common.ErrConflict
	}
	return err
}

func (r *Repo) ListSuppliers(ctx context.Context, tenantID, query string, limit, offset int) ([]proc.Supplier, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE tenant_id=$1"
	args := []any{tenantID}
	q := strings.TrimSpace(query)
	if q != "" {
		where += " AND (name ILIKE $2 OR document ILIKE $2 OR contact_name ILIKE $2)"
		args = append(args, "%"+q+"%")
	}
	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM suppliers "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id::text, name, document, email::text, phone, contact_name, notes, active,
		       created_at::text, updated_at::text
		FROM suppliers
		%s
		ORDER BY active DESC, name
		LIMIT $%d OFFSET $%d
	`, where, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]proc.Supplier, 0)
	for rows.Next() {
		var s proc.Supplier
		if err := rows.Scan(&s.ID, &s.Name, &s.Document, &s.Email, &s.Phone, &s.ContactName, &s.Notes, &s.Active, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

func (r *Repo) GetSupplier(ctx context.Context, tenantID, id string) (proc.Supplier, error) {
	var s proc.Supplier
	err := r.db.QueryRow(ctx, `
		SELECT id::text, name, document, email::text, phone, contact_name, notes, active,
		       created_at::text, updated_at::text
		FROM suppliers
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id).Scan(&s.ID, &s.Name, &s.Document, &s.Email, &s.Phone, &s.ContactName, &s.Notes, &s.Active, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repo) CreateSupplier(ctx context.Context, tx db.DBTX, tenantID string, s proc.Supplier) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO suppliers(tenant_id, name, document, email, phone, contact_name, notes, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text
	`, tenantID, s.Name, s.Document, s.Email, s.Phone, s.ContactName, s.Notes, s.Active).Scan(&id)
	return id, mapConstraintError(err)
}

func (r *Repo) UpdateSupplier(ctx context.Context, tx db.DBTX, tenantID, id string, s proc.Supplier) error {
	tag, err := tx.Exec(ctx, `
		UPDATE suppliers
		SET name=$3, document=$4, email=$5, phone=$6, contact_name=$7, notes=$8, active=$9, updated_at=now()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, s.Name, s.Document, s.Email, s.Phone, s.ContactName, s.Notes, s.Active)
	if err != nil {
		return mapConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func scanPurchase(row interface{ Scan(...any) error }) (proc.Purchase, error) {
	var p proc.Purchase
	var status string
	var total string
	err := row.Scan(
		&p.ID, &p.SupplierID, &p.SupplierName, &status, &p.InvoiceNumber, &p.PaymentDueDate,
		&total, &p.Notes, &p.CreatedBy, &p.OrderedAt, &p.ReceivedAt, &p.CancelledAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return p, err
	}
	p.Status = proc.PurchaseStatus(status)
	p.Total, err = platform.ParseMoney(total)
	return p, err
}

func purchaseSelect() string {
	return `
		SELECT p.id::text, p.supplier_id::text, s.name, p.status, p.invoice_number, p.payment_due_date::text,
		       p.total::text, p.notes, p.created_by_user_id::text, p.ordered_at::text,
		       p.received_at::text, p.cancelled_at::text, p.created_at::text, p.updated_at::text
		FROM purchases p
		JOIN suppliers s ON s.id=p.supplier_id AND s.tenant_id=p.tenant_id
	`
}

func (r *Repo) ListPurchases(ctx context.Context, tenantID, status string, limit, offset int) ([]proc.Purchase, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE p.tenant_id=$1"
	args := []any{tenantID}
	if status != "" {
		where += " AND p.status=$2"
		args = append(args, status)
	}
	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM purchases p "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf("%s %s ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d", purchaseSelect(), where, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]proc.Purchase, 0)
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, p)
	}
	return items, total, rows.Err()
}

func (r *Repo) GetPurchase(ctx context.Context, tenantID, id string) (proc.Purchase, []proc.PurchaseItem, []proc.Receipt, error) {
	p, err := scanPurchase(r.db.QueryRow(ctx, purchaseSelect()+" WHERE p.tenant_id=$1 AND p.id=$2", tenantID, id))
	if err != nil {
		return p, nil, nil, err
	}
	items, err := r.listPurchaseItems(ctx, r.db, tenantID, id, false)
	if err != nil {
		return p, nil, nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text, purchase_id::text, received_by_user_id::text, notes, received_at::text
		FROM purchase_receipts
		WHERE tenant_id=$1 AND purchase_id=$2
		ORDER BY received_at DESC
	`, tenantID, id)
	if err != nil {
		return p, nil, nil, err
	}
	defer rows.Close()
	receipts := make([]proc.Receipt, 0)
	for rows.Next() {
		var receipt proc.Receipt
		if err := rows.Scan(&receipt.ID, &receipt.PurchaseID, &receipt.ReceivedBy, &receipt.Notes, &receipt.ReceivedAt); err != nil {
			return p, nil, nil, err
		}
		receipts = append(receipts, receipt)
	}
	return p, items, receipts, rows.Err()
}

func (r *Repo) listPurchaseItems(ctx context.Context, q db.DBTX, tenantID, purchaseID string, forUpdate bool) ([]proc.PurchaseItem, error) {
	sql := `
		SELECT pi.id::text, pi.purchase_id::text, pi.product_id::text, pr.sku, pr.name,
		       pi.qty_ordered::text, pi.qty_received::text, pi.unit_cost::text, pi.line_total::text
		FROM purchase_items pi
		JOIN products pr ON pr.id=pi.product_id AND pr.tenant_id=pi.tenant_id
		WHERE pi.tenant_id=$1 AND pi.purchase_id=$2
		ORDER BY pi.created_at, pi.id
	`
	if forUpdate {
		sql += " FOR UPDATE OF pi"
	}
	rows, err := q.Query(ctx, sql, tenantID, purchaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]proc.PurchaseItem, 0)
	for rows.Next() {
		var item proc.PurchaseItem
		var ordered, received, unitCost, lineTotal string
		if err := rows.Scan(&item.ID, &item.PurchaseID, &item.ProductID, &item.ProductSKU, &item.ProductName, &ordered, &received, &unitCost, &lineTotal); err != nil {
			return nil, err
		}
		var err error
		if item.QtyOrdered, err = platform.ParseQuantity(ordered); err != nil {
			return nil, err
		}
		if item.QtyReceived, err = platform.ParseQuantity(received); err != nil {
			return nil, err
		}
		if item.UnitCost, err = platform.ParseMoney(unitCost); err != nil {
			return nil, err
		}
		if item.LineTotal, err = platform.ParseMoney(lineTotal); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) GetPurchaseForUpdate(ctx context.Context, tx db.DBTX, tenantID, id string) (proc.Purchase, []proc.PurchaseItem, error) {
	p, err := scanPurchase(tx.QueryRow(ctx, purchaseSelect()+" WHERE p.tenant_id=$1 AND p.id=$2 FOR UPDATE OF p", tenantID, id))
	if err != nil {
		return p, nil, err
	}
	items, err := r.listPurchaseItems(ctx, tx, tenantID, id, true)
	return p, items, err
}

func (r *Repo) CreatePurchase(ctx context.Context, tx db.DBTX, tenantID string, p proc.Purchase, items []proc.PurchaseItem) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO purchases(tenant_id, supplier_id, status, invoice_number, payment_due_date, total, notes, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text
	`, tenantID, p.SupplierID, string(p.Status), p.InvoiceNumber, p.PaymentDueDate, p.Total.DBString(), p.Notes, p.CreatedBy).Scan(&id)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO purchase_items(tenant_id, purchase_id, product_id, qty_ordered, unit_cost, line_total)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, tenantID, id, item.ProductID, item.QtyOrdered.DBString(), item.UnitCost.DBString(), item.LineTotal.DBString()); err != nil {
			return "", err
		}
	}
	return id, nil
}

func (r *Repo) CreateReceipt(ctx context.Context, tx db.DBTX, tenantID, purchaseID, actorUserID string, notes *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO purchase_receipts(tenant_id, purchase_id, received_by_user_id, notes)
		VALUES ($1,$2,$3,$4)
		RETURNING id::text
	`, tenantID, purchaseID, actorUserID, notes).Scan(&id)
	return id, err
}

func (r *Repo) InsertReceiptItem(ctx context.Context, tx db.DBTX, tenantID string, item proc.ReceiptItem) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO purchase_receipt_items(tenant_id, receipt_id, purchase_item_id, product_id, qty, unit_cost)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, tenantID, item.ReceiptID, item.PurchaseItemID, item.ProductID, item.Qty.DBString(), item.UnitCost.DBString())
	return err
}

func (r *Repo) UpdateItemReceived(ctx context.Context, tx db.DBTX, tenantID, itemID string, qtyReceived platform.Quantity) error {
	tag, err := tx.Exec(ctx, `
		UPDATE purchase_items SET qty_received=$3
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, itemID, qtyReceived.DBString())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repo) UpdatePurchaseStatus(ctx context.Context, tx db.DBTX, tenantID, purchaseID string, status proc.PurchaseStatus, receivedAt *string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE purchases
		SET status=$3, received_at=CASE WHEN $4::text IS NULL THEN received_at ELSE $4::timestamptz END, updated_at=now()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, purchaseID, string(status), receivedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repo) CancelPurchase(ctx context.Context, tx db.DBTX, tenantID, purchaseID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE purchases
		SET status='cancelled', cancelled_at=now(), updated_at=now()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, purchaseID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repo) UpdateProductCost(ctx context.Context, tx db.DBTX, tenantID, productID string, cost platform.Money) error {
	tag, err := tx.Exec(ctx, `
		UPDATE products SET cost_price=$3, updated_at=now()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, productID, cost.DBString())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *Repo) CreateAccountPayable(ctx context.Context, tx db.DBTX, tenantID, purchaseID, supplierID string, description string, amount platform.Money, dueDate string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO accounts_payable(tenant_id, description, amount, due_date, status, supplier_id, purchase_id)
		VALUES ($1,$2,$3,$4::date,'open',$5,$6)
		ON CONFLICT (tenant_id, purchase_id) WHERE purchase_id IS NOT NULL DO NOTHING
	`, tenantID, description, amount.DBString(), dueDate, supplierID, purchaseID)
	return err
}

func (r *Repo) CancelAccountPayable(ctx context.Context, tx db.DBTX, tenantID, purchaseID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE accounts_payable SET status='cancelled'
		WHERE tenant_id=$1 AND purchase_id=$2 AND status='open'
	`, tenantID, purchaseID)
	return err
}
