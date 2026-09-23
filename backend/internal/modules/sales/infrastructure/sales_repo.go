package infrastructure

import (
	"context"

	"github.com/example/sistemaemgo/internal/modules/common"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SalesRepo struct {
	db *pgxpool.Pool
}

func NewSalesRepo(dbpool *pgxpool.Pool) *SalesRepo {
	return &SalesRepo{db: dbpool}
}

func (r *SalesRepo) InsertSale(ctx context.Context, tx db.DBTX, tenantID string, s sales.Sale) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO sales(tenant_id, cash_session_id, customer_id, status, subtotal, discount_value, total, profit_estimated, created_by_user_id, cancel_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id::text
	`, tenantID, s.CashSessionID, s.CustomerID, s.Status, s.Subtotal.DBString(), s.DiscountValue.DBString(), s.Total.DBString(), s.ProfitEstimated.DBString(), s.CreatedByUserID, s.CancelReason).Scan(&id)
	return id, err
}

func (r *SalesRepo) InsertItem(ctx context.Context, tx db.DBTX, tenantID string, it sales.SaleItem) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sale_items(tenant_id, sale_id, product_id, qty, unit_price, discount_value, subtotal, cost_unit)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, tenantID, it.SaleID, it.ProductID, it.Qty.DBString(), it.UnitPrice.DBString(), it.DiscountValue.DBString(), it.Subtotal.DBString(), it.CostUnit.DBString())
	return err
}

func (r *SalesRepo) InsertPayment(ctx context.Context, tx db.DBTX, tenantID string, p sales.Payment) error {
	installments := p.Installments
	if installments <= 0 {
		installments = 1
	}
	status := p.ReconciliationStatus
	if status == "" {
		status = "pending"
		if p.Method == "cash" {
			status = "not_applicable"
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO payments(
			tenant_id, sale_id, method, amount, provider, transaction_ref,
			authorization_code, installments, reconciliation_status
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, tenantID, p.SaleID, p.Method, p.Amount.DBString(), p.Provider, p.TransactionRef, p.AuthorizationCode, installments, status)
	return err
}

func (r *SalesRepo) GetSale(ctx context.Context, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	var s sales.Sale
	var subtotal, discount, total, profit string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::text, discount_value::text, total::text, profit_estimated::text, created_by_user_id::text, cancel_reason
		FROM sales WHERE tenant_id=$1 AND id=$2
	`, tenantID, id).Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &subtotal, &discount, &total, &profit, &s.CreatedByUserID, &s.CancelReason)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	if err := assignSaleMoney(&s, subtotal, discount, total, profit); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	itemsRows, err := r.db.Query(ctx, `
		SELECT id::text, sale_id::text, product_id::text, qty::text, unit_price::text, discount_value::text, subtotal::text, cost_unit::text
		FROM sale_items WHERE tenant_id=$1 AND sale_id=$2 ORDER BY created_at
	`, tenantID, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer itemsRows.Close()
	var items []sales.SaleItem
	for itemsRows.Next() {
		var it sales.SaleItem
		var qty, unitPrice, discountValue, itemSubtotal, costUnit string
		if err := itemsRows.Scan(&it.ID, &it.SaleID, &it.ProductID, &qty, &unitPrice, &discountValue, &itemSubtotal, &costUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		if err := assignSaleItemNumbers(&it, qty, unitPrice, discountValue, itemSubtotal, costUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		items = append(items, it)
	}
	if err := itemsRows.Err(); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	payRows, err := r.db.Query(ctx, `
		SELECT id::text, sale_id::text, method, amount::text, provider, transaction_ref,
		       authorization_code, installments, reconciliation_status
		FROM payments
		WHERE tenant_id=$1 AND sale_id=$2
		ORDER BY created_at
	`, tenantID, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer payRows.Close()
	var pays []sales.Payment
	for payRows.Next() {
		var p sales.Payment
		var amount string
		if err := payRows.Scan(
			&p.ID, &p.SaleID, &p.Method, &amount, &p.Provider, &p.TransactionRef,
			&p.AuthorizationCode, &p.Installments, &p.ReconciliationStatus,
		); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		if p.Amount, err = platform.ParseMoney(amount); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		pays = append(pays, p)
	}
	return s, items, pays, payRows.Err()
}

func (r *SalesRepo) ListSales(ctx context.Context, tenantID string, limit, offset int) ([]sales.Sale, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM sales WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::text, discount_value::text, total::text, profit_estimated::text, created_by_user_id::text, cancel_reason
		FROM sales
		WHERE tenant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []sales.Sale
	for rows.Next() {
		var s sales.Sale
		var subtotal, discount, total, profit string
		if err := rows.Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &subtotal, &discount, &total, &profit, &s.CreatedByUserID, &s.CancelReason); err != nil {
			return nil, 0, err
		}
		if err := assignSaleMoney(&s, subtotal, discount, total, profit); err != nil {
			return nil, 0, err
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

func (r *SalesRepo) HasInvoiceForSale(ctx context.Context, tx db.DBTX, tenantID string, id string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM invoices WHERE tenant_id=$1 AND sale_id=$2
		)
	`, tenantID, id).Scan(&exists)
	return exists, err
}

func (r *SalesRepo) HasReturnsForSale(ctx context.Context, tx db.DBTX, tenantID string, id string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sale_returns WHERE tenant_id=$1 AND sale_id=$2
		)
	`, tenantID, id).Scan(&exists)
	return exists, err
}

func (r *SalesRepo) CancelSale(ctx context.Context, tx db.DBTX, tenantID string, id string, reason string) error {
	_, err := tx.Exec(ctx, `
		UPDATE sales
		SET status='cancelled', cancelled_at=now(), cancel_reason=$2
		WHERE tenant_id=$1 AND id=$3 AND status='finalized'
	`, tenantID, reason, id)
	return err
}

func (r *SalesRepo) GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	var s sales.Sale
	var subtotal, discount, total, profit string
	err := tx.QueryRow(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::text, discount_value::text, total::text, profit_estimated::text, created_by_user_id::text, cancel_reason
		FROM sales WHERE tenant_id=$1 AND id=$2 FOR UPDATE
	`, tenantID, id).Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &subtotal, &discount, &total, &profit, &s.CreatedByUserID, &s.CancelReason)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	if err := assignSaleMoney(&s, subtotal, discount, total, profit); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	itemsRows, err := tx.Query(ctx, `
		SELECT id::text, sale_id::text, product_id::text, qty::text, unit_price::text, discount_value::text, subtotal::text, cost_unit::text
		FROM sale_items WHERE tenant_id=$1 AND sale_id=$2 ORDER BY created_at
	`, tenantID, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer itemsRows.Close()
	var items []sales.SaleItem
	for itemsRows.Next() {
		var it sales.SaleItem
		var qty, unitPrice, discountValue, itemSubtotal, costUnit string
		if err := itemsRows.Scan(&it.ID, &it.SaleID, &it.ProductID, &qty, &unitPrice, &discountValue, &itemSubtotal, &costUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		if err := assignSaleItemNumbers(&it, qty, unitPrice, discountValue, itemSubtotal, costUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		items = append(items, it)
	}
	if err := itemsRows.Err(); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	payRows, err := tx.Query(ctx, `
		SELECT id::text, sale_id::text, method, amount::text, provider, transaction_ref,
		       authorization_code, installments, reconciliation_status
		FROM payments
		WHERE tenant_id=$1 AND sale_id=$2
		ORDER BY created_at
	`, tenantID, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer payRows.Close()
	var pays []sales.Payment
	for payRows.Next() {
		var p sales.Payment
		var amount string
		if err := payRows.Scan(
			&p.ID, &p.SaleID, &p.Method, &amount, &p.Provider, &p.TransactionRef,
			&p.AuthorizationCode, &p.Installments, &p.ReconciliationStatus,
		); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		if p.Amount, err = platform.ParseMoney(amount); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		pays = append(pays, p)
	}
	return s, items, pays, payRows.Err()
}

func (r *SalesRepo) LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error {
	lockKey := tenantID + ":" + operation + ":" + key
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, lockKey)
	return err
}

func (r *SalesRepo) GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (saleID string, total platform.Money, requestHash string, ok bool, err error) {
	var totalRaw string
	err = tx.QueryRow(ctx, `
		SELECT sale_id::text, total::text, request_hash
		FROM idempotency_keys
		WHERE tenant_id=$1 AND operation=$2 AND idem_key=$3
	`, tenantID, operation, key).Scan(&saleID, &totalRaw, &requestHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", 0, "", false, nil
		}
		return "", 0, "", false, err
	}
	total, err = platform.ParseMoney(totalRaw)
	if err != nil {
		return "", 0, "", false, err
	}
	return saleID, total, requestHash, true, nil
}

func (r *SalesRepo) SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, saleID string, total platform.Money) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_keys(tenant_id, operation, idem_key, request_hash, sale_id, total)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, operation, idem_key)
		DO NOTHING
	`, tenantID, operation, key, requestHash, saleID, total.DBString())
	if err == nil && tag.RowsAffected() == 0 {
		return common.ErrConflict
	}
	return err
}

func assignSaleMoney(s *sales.Sale, subtotal, discount, total, profit string) error {
	var err error
	if s.Subtotal, err = platform.ParseMoney(subtotal); err != nil {
		return err
	}
	if s.DiscountValue, err = platform.ParseMoney(discount); err != nil {
		return err
	}
	if s.Total, err = platform.ParseMoney(total); err != nil {
		return err
	}
	if s.ProfitEstimated, err = platform.ParseMoney(profit); err != nil {
		return err
	}
	return nil
}

func assignSaleItemNumbers(it *sales.SaleItem, qty, unitPrice, discount, subtotal, costUnit string) error {
	var err error
	if it.Qty, err = platform.ParseQuantity(qty); err != nil {
		return err
	}
	if it.UnitPrice, err = platform.ParseMoney(unitPrice); err != nil {
		return err
	}
	if it.DiscountValue, err = platform.ParseMoney(discount); err != nil {
		return err
	}
	if it.Subtotal, err = platform.ParseMoney(subtotal); err != nil {
		return err
	}
	if it.CostUnit, err = platform.ParseMoney(costUnit); err != nil {
		return err
	}
	return nil
}
