package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/sistemaemgo/internal/modules/common"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{db: pool} }

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return common.ErrNotFound
	}
	return err
}

func (r *Repo) List(ctx context.Context, tenantID, saleID string, limit, offset int) ([]ret.SaleReturn, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE tenant_id=$1"
	args := []any{tenantID}
	if saleID != "" {
		where += " AND sale_id=$2"
		args = append(args, saleID)
	}
	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM sale_returns "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id::text, sale_id::text, kind, reason, refund_due::text,
		       replacement_sale_id::text, created_by_user_id::text, created_at::text
		FROM sale_returns
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, where, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ret.SaleReturn, 0)
	for rows.Next() {
		var item ret.SaleReturn
		var refund string
		if err := rows.Scan(&item.ID, &item.SaleID, &item.Kind, &item.Reason, &refund, &item.ReplacementSaleID, &item.CreatedByUserID, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.RefundDue, err = platform.ParseMoney(refund)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (r *Repo) Get(ctx context.Context, tenantID, id string) (ret.SaleReturn, []ret.Item, error) {
	var out ret.SaleReturn
	var refund string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, sale_id::text, kind, reason, refund_due::text,
		       replacement_sale_id::text, created_by_user_id::text, created_at::text
		FROM sale_returns
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id).Scan(&out.ID, &out.SaleID, &out.Kind, &out.Reason, &refund, &out.ReplacementSaleID, &out.CreatedByUserID, &out.CreatedAt)
	if err != nil {
		return out, nil, mapErr(err)
	}
	out.RefundDue, err = platform.ParseMoney(refund)
	if err != nil {
		return out, nil, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text, return_id::text, sale_item_id::text, product_id::text,
		       qty::text, restock, refund_value::text, created_at::text
		FROM sale_return_items
		WHERE tenant_id=$1 AND return_id=$2
		ORDER BY created_at, id
	`, tenantID, id)
	if err != nil {
		return out, nil, err
	}
	defer rows.Close()
	items := make([]ret.Item, 0)
	for rows.Next() {
		var item ret.Item
		var qty, value string
		if err := rows.Scan(&item.ID, &item.ReturnID, &item.SaleItemID, &item.ProductID, &qty, &item.Restock, &value, &item.CreatedAt); err != nil {
			return out, nil, err
		}
		item.Qty, err = platform.ParseQuantity(qty)
		if err != nil {
			return out, nil, err
		}
		item.RefundValue, err = platform.ParseMoney(value)
		if err != nil {
			return out, nil, err
		}
		items = append(items, item)
	}
	return out, items, rows.Err()
}

func (r *Repo) GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID, saleID string) (sales.Sale, []sales.SaleItem, error) {
	var s sales.Sale
	var subtotal, discount, total, profit string
	err := tx.QueryRow(ctx, `
		SELECT id::text, cash_session_id::text, customer_id::text, status,
		       subtotal::text, discount_value::text, total::text, profit_estimated::text,
		       created_by_user_id::text, cancel_reason
		FROM sales
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE
	`, tenantID, saleID).Scan(&s.ID, &s.CashSessionID, &s.CustomerID, &s.Status, &subtotal, &discount, &total, &profit, &s.CreatedByUserID, &s.CancelReason)
	if err != nil {
		return s, nil, mapErr(err)
	}
	if s.Subtotal, err = platform.ParseMoney(subtotal); err != nil {
		return s, nil, err
	}
	if s.DiscountValue, err = platform.ParseMoney(discount); err != nil {
		return s, nil, err
	}
	if s.Total, err = platform.ParseMoney(total); err != nil {
		return s, nil, err
	}
	if s.ProfitEstimated, err = platform.ParseMoney(profit); err != nil {
		return s, nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, sale_id::text, product_id::text, qty::text,
		       unit_price::text, discount_value::text, subtotal::text, cost_unit::text
		FROM sale_items
		WHERE tenant_id=$1 AND sale_id=$2
		ORDER BY created_at, id
	`, tenantID, saleID)
	if err != nil {
		return s, nil, err
	}
	defer rows.Close()
	items := make([]sales.SaleItem, 0)
	for rows.Next() {
		var item sales.SaleItem
		var qty, unitPrice, discountValue, lineSubtotal, costUnit string
		if err := rows.Scan(&item.ID, &item.SaleID, &item.ProductID, &qty, &unitPrice, &discountValue, &lineSubtotal, &costUnit); err != nil {
			return s, nil, err
		}
		if item.Qty, err = platform.ParseQuantity(qty); err != nil {
			return s, nil, err
		}
		if item.UnitPrice, err = platform.ParseMoney(unitPrice); err != nil {
			return s, nil, err
		}
		if item.DiscountValue, err = platform.ParseMoney(discountValue); err != nil {
			return s, nil, err
		}
		if item.Subtotal, err = platform.ParseMoney(lineSubtotal); err != nil {
			return s, nil, err
		}
		if item.CostUnit, err = platform.ParseMoney(costUnit); err != nil {
			return s, nil, err
		}
		items = append(items, item)
	}
	return s, items, rows.Err()
}

func (r *Repo) SumReturnedBySaleItem(ctx context.Context, tx db.DBTX, tenantID string, saleItemIDs []string) (map[string]platform.Quantity, error) {
	out := make(map[string]platform.Quantity, len(saleItemIDs))
	if len(saleItemIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT sale_item_id::text, COALESCE(sum(qty),0)::text
		FROM sale_return_items
		WHERE tenant_id=$1 AND sale_item_id = ANY($2::uuid[])
		GROUP BY sale_item_id
	`, tenantID, saleItemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		qty, err := platform.ParseQuantity(raw)
		if err != nil {
			return nil, err
		}
		out[id] = qty
	}
	return out, rows.Err()
}

func (r *Repo) SumRefundDue(ctx context.Context, tx db.DBTX, tenantID, saleID string) (platform.Money, error) {
	var raw string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(refund_due),0)::text
		FROM sale_returns
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenantID, saleID).Scan(&raw); err != nil {
		return 0, err
	}
	return platform.ParseMoney(raw)
}

func (r *Repo) CreateReturn(ctx context.Context, tx db.DBTX, tenantID string, item ret.SaleReturn) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO sale_returns(tenant_id, sale_id, kind, reason, refund_due, replacement_sale_id, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id::text
	`, tenantID, item.SaleID, string(item.Kind), item.Reason, item.RefundDue.DBString(), item.ReplacementSaleID, item.CreatedByUserID).Scan(&id)
	return id, err
}

func (r *Repo) InsertItem(ctx context.Context, tx db.DBTX, tenantID string, item ret.Item) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sale_return_items(tenant_id, return_id, sale_item_id, product_id, qty, restock, refund_value)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, tenantID, item.ReturnID, item.SaleItemID, item.ProductID, item.Qty.DBString(), item.Restock, item.RefundValue.DBString())
	return err
}

func (r *Repo) LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, tenantID+":"+operation+":"+key)
	return err
}

func (r *Repo) GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (returnID, requestHash string, refundDue platform.Money, ok bool, err error) {
	var raw string
	err = tx.QueryRow(ctx, `
		SELECT return_id::text, request_hash, refund_due::text
		FROM return_idempotency_keys
		WHERE tenant_id=$1 AND operation=$2 AND idem_key=$3
	`, tenantID, operation, key).Scan(&returnID, &requestHash, &raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", 0, false, nil
		}
		return "", "", 0, false, err
	}
	refundDue, err = platform.ParseMoney(raw)
	if err != nil {
		return "", "", 0, false, err
	}
	return returnID, requestHash, refundDue, true, nil
}

func (r *Repo) SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, returnID string, refundDue platform.Money) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO return_idempotency_keys(tenant_id, operation, idem_key, request_hash, return_id, refund_due)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, operation, idem_key) DO NOTHING
	`, tenantID, operation, key, requestHash, returnID, refundDue.DBString())
	if err == nil && tag.RowsAffected() == 0 {
		return common.ErrConflict
	}
	return err
}
