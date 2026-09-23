package infrastructure

import (
	"context"
	"fmt"

	"github.com/example/sistemaemgo/internal/modules/common"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{db: pool}
}

func scanReturn(row interface{ Scan(...any) error }) (ret.Return, error) {
	var value ret.Return
	var total, recovered, refunded string
	err := row.Scan(
		&value.ID, &value.SaleID, &value.Reason, &total, &recovered, &refunded,
		&value.CreatedBy, &value.CreatedAt,
	)
	if err != nil {
		return value, err
	}
	if value.TotalAmount, err = platform.ParseMoney(total); err != nil {
		return value, err
	}
	if value.RecoveredCost, err = platform.ParseMoney(recovered); err != nil {
		return value, err
	}
	if value.RefundedAmount, err = platform.ParseMoney(refunded); err != nil {
		return value, err
	}
	return value, nil
}

func returnSelect() string {
	return `
		SELECT r.id::text, r.sale_id::text, r.reason, r.total_amount::text, r.recovered_cost::text,
		       COALESCE((SELECT SUM(f.amount) FROM sale_refunds f
		                 WHERE f.tenant_id=r.tenant_id AND f.return_id=r.id), 0)::text,
		       r.created_by_user_id::text, r.created_at::text
		FROM sale_returns r
	`
}

func (r *Repo) ListReturns(ctx context.Context, tenantID, saleID string, limit, offset int) ([]ret.Return, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE r.tenant_id=$1"
	args := []any{tenantID}
	if saleID != "" {
		where += " AND r.sale_id=$2"
		args = append(args, saleID)
	}

	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM sale_returns r "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf("%s %s ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d", returnSelect(), where, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]ret.Return, 0)
	for rows.Next() {
		value, err := scanReturn(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, value)
	}
	return items, total, rows.Err()
}

func (r *Repo) GetReturn(ctx context.Context, tenantID, returnID string) (ret.Return, []ret.ReturnItem, []ret.Refund, error) {
	value, err := scanReturn(r.db.QueryRow(ctx, returnSelect()+" WHERE r.tenant_id=$1 AND r.id=$2", tenantID, returnID))
	if err != nil {
		return value, nil, nil, err
	}

	itemRows, err := r.db.Query(ctx, `
		SELECT id::text, return_id::text, sale_item_id::text, product_id::text,
		       qty::text, restock, amount::text, recovered_cost::text
		FROM sale_return_items
		WHERE tenant_id=$1 AND return_id=$2
		ORDER BY created_at, id
	`, tenantID, returnID)
	if err != nil {
		return value, nil, nil, err
	}
	defer itemRows.Close()

	items := make([]ret.ReturnItem, 0)
	for itemRows.Next() {
		var item ret.ReturnItem
		var qty, amount, recovered string
		if err := itemRows.Scan(&item.ID, &item.ReturnID, &item.SaleItemID, &item.ProductID, &qty, &item.Restock, &amount, &recovered); err != nil {
			return value, nil, nil, err
		}
		if item.Qty, err = platform.ParseQuantity(qty); err != nil {
			return value, nil, nil, err
		}
		if item.Amount, err = platform.ParseMoney(amount); err != nil {
			return value, nil, nil, err
		}
		if item.RecoveredCost, err = platform.ParseMoney(recovered); err != nil {
			return value, nil, nil, err
		}
		items = append(items, item)
	}
	if err := itemRows.Err(); err != nil {
		return value, nil, nil, err
	}

	refundRows, err := r.db.Query(ctx, `
		SELECT id::text, return_id::text, sale_id::text, method, amount::text,
		       external_reference, notes, created_by_user_id::text, created_at::text
		FROM sale_refunds
		WHERE tenant_id=$1 AND return_id=$2
		ORDER BY created_at, id
	`, tenantID, returnID)
	if err != nil {
		return value, nil, nil, err
	}
	defer refundRows.Close()

	refunds := make([]ret.Refund, 0)
	for refundRows.Next() {
		refund, err := scanRefund(refundRows)
		if err != nil {
			return value, nil, nil, err
		}
		refunds = append(refunds, refund)
	}
	return value, items, refunds, refundRows.Err()
}

func (r *Repo) GetReturnForUpdate(ctx context.Context, tx db.DBTX, tenantID, returnID string) (ret.Return, error) {
	return scanReturn(tx.QueryRow(ctx, returnSelect()+" WHERE r.tenant_id=$1 AND r.id=$2 FOR UPDATE OF r", tenantID, returnID))
}

func scanRefund(row interface{ Scan(...any) error }) (ret.Refund, error) {
	var value ret.Refund
	var amount string
	err := row.Scan(
		&value.ID, &value.ReturnID, &value.SaleID, &value.Method, &amount,
		&value.ExternalReference, &value.Notes, &value.CreatedBy, &value.CreatedAt,
	)
	if err != nil {
		return value, err
	}
	value.Amount, err = platform.ParseMoney(amount)
	return value, err
}

func (r *Repo) GetRefund(ctx context.Context, tenantID, refundID string) (ret.Refund, error) {
	return scanRefund(r.db.QueryRow(ctx, `
		SELECT id::text, return_id::text, sale_id::text, method, amount::text,
		       external_reference, notes, created_by_user_id::text, created_at::text
		FROM sale_refunds
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, refundID))
}

func (r *Repo) GetReturnedAggregates(ctx context.Context, tx db.DBTX, tenantID, saleID string) (map[string]ret.ReturnedAggregate, error) {
	rows, err := tx.Query(ctx, `
		SELECT ri.sale_item_id::text, COALESCE(SUM(ri.qty),0)::text, COALESCE(SUM(ri.amount),0)::text
		FROM sale_return_items ri
		JOIN sale_returns r ON r.id=ri.return_id AND r.tenant_id=ri.tenant_id
		WHERE ri.tenant_id=$1 AND r.sale_id=$2
		GROUP BY ri.sale_item_id
	`, tenantID, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]ret.ReturnedAggregate{}
	for rows.Next() {
		var saleItemID, qtyRaw, amountRaw string
		if err := rows.Scan(&saleItemID, &qtyRaw, &amountRaw); err != nil {
			return nil, err
		}
		qty, err := platform.ParseQuantity(qtyRaw)
		if err != nil {
			return nil, err
		}
		amount, err := platform.ParseMoney(amountRaw)
		if err != nil {
			return nil, err
		}
		out[saleItemID] = ret.ReturnedAggregate{Qty: qty, Amount: amount}
	}
	return out, rows.Err()
}

func (r *Repo) CreateReturn(ctx context.Context, tx db.DBTX, tenantID string, value ret.Return) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO sale_returns(
			tenant_id, sale_id, reason, total_amount, recovered_cost, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id::text
	`, tenantID, value.SaleID, value.Reason, value.TotalAmount.DBString(), value.RecoveredCost.DBString(), value.CreatedBy).Scan(&id)
	return id, err
}

func (r *Repo) InsertReturnItem(ctx context.Context, tx db.DBTX, tenantID string, item ret.ReturnItem) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO sale_return_items(
			tenant_id, return_id, sale_item_id, product_id, qty, restock, amount, recovered_cost
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, tenantID, item.ReturnID, item.SaleItemID, item.ProductID, item.Qty.DBString(), item.Restock, item.Amount.DBString(), item.RecoveredCost.DBString())
	return err
}

func (r *Repo) CreateRefund(ctx context.Context, tx db.DBTX, tenantID string, value ret.Refund) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO sale_refunds(
			tenant_id, return_id, sale_id, method, amount, external_reference, notes, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text
	`, tenantID, value.ReturnID, value.SaleID, value.Method, value.Amount.DBString(), value.ExternalReference, value.Notes, value.CreatedBy).Scan(&id)
	return id, err
}

func (r *Repo) LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error {
	lockKey := tenantID + ":" + operation + ":" + key
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, lockKey)
	return err
}

func (r *Repo) GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (resourceID, resultStatus, requestHash string, ok bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT resource_id::text, COALESCE(result_status, ''), request_hash
		FROM sales_return_idempotency_keys
		WHERE tenant_id=$1 AND operation=$2 AND idem_key=$3
	`, tenantID, operation, key).Scan(&resourceID, &resultStatus, &requestHash)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", "", false, nil
		}
		return "", "", "", false, err
	}
	return resourceID, resultStatus, requestHash, true, nil
}

func (r *Repo) SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, resourceID, resultStatus string) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO sales_return_idempotency_keys(
			tenant_id, operation, idem_key, request_hash, resource_id, result_status
		)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, operation, idem_key)
		DO NOTHING
	`, tenantID, operation, key, requestHash, resourceID, resultStatus)
	if err == nil && tag.RowsAffected() == 0 {
		return common.ErrConflict
	}
	return err
}
