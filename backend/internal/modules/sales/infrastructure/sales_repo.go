package infrastructure

import (
	"context"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SalesRepo struct {
	db *pgxpool.Pool
}

func NewSalesRepo(dbpool *pgxpool.Pool) *SalesRepo {
	return &SalesRepo{db: dbpool}
}

func (r *SalesRepo) InsertSale(ctx context.Context, tx db.DBTX, s sales.Sale) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO sales(cash_session_id, customer_id, status, subtotal, discount_value, total, profit_estimated, created_by_user_id, cancel_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id::text
	`, s.CashSessionID, s.CustomerID, s.Status, s.Subtotal, s.DiscountValue, s.Total, s.ProfitEstimated, s.CreatedByUserID, s.CancelReason).Scan(&id)
	return id, err
}

func (r *SalesRepo) InsertItem(ctx context.Context, tx db.DBTX, it sales.SaleItem) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sale_items(sale_id, product_id, qty, unit_price, discount_value, subtotal, cost_unit)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, it.SaleID, it.ProductID, it.Qty, it.UnitPrice, it.DiscountValue, it.Subtotal, it.CostUnit)
	return err
}

func (r *SalesRepo) InsertPayment(ctx context.Context, tx db.DBTX, p sales.Payment) error {
	_, err := tx.Exec(ctx, `INSERT INTO payments(sale_id, method, amount) VALUES ($1,$2,$3)`, p.SaleID, p.Method, p.Amount)
	return err
}

func (r *SalesRepo) GetSale(ctx context.Context, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	var s sales.Sale
	err := r.db.QueryRow(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::float8, discount_value::float8, total::float8, profit_estimated::float8, created_by_user_id::text, cancel_reason
		FROM sales WHERE id=$1
	`, id).Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &s.Subtotal, &s.DiscountValue, &s.Total, &s.ProfitEstimated, &s.CreatedByUserID, &s.CancelReason)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}

	itemsRows, err := r.db.Query(ctx, `
		SELECT id::text, sale_id::text, product_id::text, qty::float8, unit_price::float8, discount_value::float8, subtotal::float8, cost_unit::float8
		FROM sale_items WHERE sale_id=$1 ORDER BY created_at
	`, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer itemsRows.Close()
	var items []sales.SaleItem
	for itemsRows.Next() {
		var it sales.SaleItem
		if err := itemsRows.Scan(&it.ID, &it.SaleID, &it.ProductID, &it.Qty, &it.UnitPrice, &it.DiscountValue, &it.Subtotal, &it.CostUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		items = append(items, it)
	}
	if err := itemsRows.Err(); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	payRows, err := r.db.Query(ctx, `SELECT id::text, sale_id::text, method, amount::float8 FROM payments WHERE sale_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer payRows.Close()
	var pays []sales.Payment
	for payRows.Next() {
		var p sales.Payment
		if err := payRows.Scan(&p.ID, &p.SaleID, &p.Method, &p.Amount); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		pays = append(pays, p)
	}
	return s, items, pays, payRows.Err()
}

func (r *SalesRepo) ListSales(ctx context.Context, limit, offset int) ([]sales.Sale, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM sales`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::float8, discount_value::float8, total::float8, profit_estimated::float8, created_by_user_id::text, cancel_reason
		FROM sales
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []sales.Sale
	for rows.Next() {
		var s sales.Sale
		if err := rows.Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &s.Subtotal, &s.DiscountValue, &s.Total, &s.ProfitEstimated, &s.CreatedByUserID, &s.CancelReason); err != nil {
			return nil, 0, err
		}
		items = append(items, s)
	}
	return items, total, rows.Err()
}

func (r *SalesRepo) CancelSale(ctx context.Context, tx db.DBTX, id string, reason string) error {
	_, err := tx.Exec(ctx, `
		UPDATE sales
		SET status='cancelled', cancelled_at=now(), cancel_reason=$2
		WHERE id=$1 AND status='finalized'
	`, id, reason)
	return err
}

func (r *SalesRepo) GetSaleForUpdate(ctx context.Context, tx db.DBTX, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	var s sales.Sale
	err := tx.QueryRow(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status, subtotal::float8, discount_value::float8, total::float8, profit_estimated::float8, created_by_user_id::text, cancel_reason
		FROM sales WHERE id=$1 FOR UPDATE
	`, id).Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &s.Subtotal, &s.DiscountValue, &s.Total, &s.ProfitEstimated, &s.CreatedByUserID, &s.CancelReason)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}

	itemsRows, err := tx.Query(ctx, `
		SELECT id::text, sale_id::text, product_id::text, qty::float8, unit_price::float8, discount_value::float8, subtotal::float8, cost_unit::float8
		FROM sale_items WHERE sale_id=$1 ORDER BY created_at
	`, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer itemsRows.Close()
	var items []sales.SaleItem
	for itemsRows.Next() {
		var it sales.SaleItem
		if err := itemsRows.Scan(&it.ID, &it.SaleID, &it.ProductID, &it.Qty, &it.UnitPrice, &it.DiscountValue, &it.Subtotal, &it.CostUnit); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		items = append(items, it)
	}
	if err := itemsRows.Err(); err != nil {
		return sales.Sale{}, nil, nil, err
	}

	payRows, err := tx.Query(ctx, `SELECT id::text, sale_id::text, method, amount::float8 FROM payments WHERE sale_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return sales.Sale{}, nil, nil, err
	}
	defer payRows.Close()
	var pays []sales.Payment
	for payRows.Next() {
		var p sales.Payment
		if err := payRows.Scan(&p.ID, &p.SaleID, &p.Method, &p.Amount); err != nil {
			return sales.Sale{}, nil, nil, err
		}
		pays = append(pays, p)
	}
	return s, items, pays, payRows.Err()
}
