package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FinanceRepo struct {
	db *pgxpool.Pool
}

func NewFinanceRepo(dbpool *pgxpool.Pool) *FinanceRepo {
	return &FinanceRepo{db: dbpool}
}

func (r *FinanceRepo) InsertLedgerEntry(ctx context.Context, tx db.DBTX, tenantID string, e fin.LedgerEntry, createdByUserID *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_entries(tenant_id, entry_type, sale_id, cash_session_id, amount_gross, amount_discount, amount_net, profit_estimated, notes, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id::text
	`, tenantID, e.EntryType, e.SaleID, e.CashSessionID, e.AmountGross.DBString(), e.AmountDiscount.DBString(), e.AmountNet.DBString(), e.ProfitEstimated.DBString(), e.Notes, createdByUserID).Scan(&id)
	return id, err
}

func (r *FinanceRepo) Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]platform.Money, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entry_type, COALESCE(SUM(amount_net),0)::text
		FROM ledger_entries
		WHERE tenant_id=$1 AND created_at >= $2::timestamptz AND created_at < ($3::timestamptz + interval '1 day')
		GROUP BY entry_type
	`, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]platform.Money{}
	for rows.Next() {
		var t string
		var raw string
		if err := rows.Scan(&t, &raw); err != nil {
			return nil, err
		}
		s, err := platform.ParseMoney(raw)
		if err != nil {
			return nil, err
		}
		out[t] = s
	}
	return out, rows.Err()
}

func (r *FinanceRepo) ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM ledger_entries WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, entry_type, sale_id::text, cash_session_id::text,
		       amount_gross::text, amount_discount::text, amount_net::text, profit_estimated::text,
		       notes, created_at::text
		FROM ledger_entries
		WHERE tenant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []fin.LedgerEntry
	for rows.Next() {
		var e fin.LedgerEntry
		var saleID *string
		var cashID *string
		var gross, discount, net, profit string
		if err := rows.Scan(&e.ID, &e.EntryType, &saleID, &cashID, &gross, &discount, &net, &profit, &e.Notes, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		var err error
		if e.AmountGross, err = platform.ParseMoney(gross); err != nil {
			return nil, 0, err
		}
		if e.AmountDiscount, err = platform.ParseMoney(discount); err != nil {
			return nil, 0, err
		}
		if e.AmountNet, err = platform.ParseMoney(net); err != nil {
			return nil, 0, err
		}
		if e.ProfitEstimated, err = platform.ParseMoney(profit); err != nil {
			return nil, 0, err
		}
		e.SaleID = saleID
		e.CashSessionID = cashID
		items = append(items, e)
	}
	return items, total, rows.Err()
}


func (r *FinanceRepo) ListPayments(ctx context.Context, tenantID, from, to, method, status string, limit, offset int) ([]fin.PaymentRecord, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := []string{"p.tenant_id=$1"}
	args := []any{tenantID}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if from != "" {
		add("p.created_at >= $%d::timestamptz", from)
	}
	if to != "" {
		add("p.created_at < ($%d::timestamptz + interval '1 day')", to)
	}
	if method != "" {
		add("p.method=$%d", method)
	}
	if status != "" {
		add("p.reconciliation_status=$%d", status)
	}
	whereSQL := ""
	for i, clause := range where {
		if i == 0 {
			whereSQL = clause
		} else {
			whereSQL += " AND " + clause
		}
	}
	var total int
	if err := r.db.QueryRow(ctx, "SELECT count(*) FROM payments p WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args) + 1
	offsetIdx := len(args) + 2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT p.id::text, p.sale_id::text, p.method, p.amount::text,
		       p.provider, p.transaction_ref, p.authorization_code, p.installments,
		       p.reconciliation_status, p.reconciled_amount::text, p.reconciled_fee::text,
		       p.reconciled_at::text, p.reconciliation_notes, p.created_at::text
		FROM payments p
		WHERE %s
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]fin.PaymentRecord, 0)
	for rows.Next() {
		var item fin.PaymentRecord
		var amount string
		var reconciledAmount, reconciledFee *string
		if err := rows.Scan(
			&item.ID, &item.SaleID, &item.Method, &amount,
			&item.Provider, &item.TransactionRef, &item.AuthorizationCode, &item.Installments,
			&item.ReconciliationStatus, &reconciledAmount, &reconciledFee,
			&item.ReconciledAt, &item.ReconciliationNotes, &item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		var err error
		if item.Amount, err = platform.ParseMoney(amount); err != nil {
			return nil, 0, err
		}
		if reconciledAmount != nil {
			v, err := platform.ParseMoney(*reconciledAmount)
			if err != nil {
				return nil, 0, err
			}
			item.ReconciledAmount = &v
		}
		if reconciledFee != nil {
			v, err := platform.ParseMoney(*reconciledFee)
			if err != nil {
				return nil, 0, err
			}
			item.ReconciledFee = &v
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *FinanceRepo) GetPaymentForUpdate(ctx context.Context, tx db.DBTX, tenantID, paymentID string) (fin.PaymentRecord, error) {
	var item fin.PaymentRecord
	var amount string
	var reconciledAmount, reconciledFee *string
	err := tx.QueryRow(ctx, `
		SELECT id::text, sale_id::text, method, amount::text,
		       provider, transaction_ref, authorization_code, installments,
		       reconciliation_status, reconciled_amount::text, reconciled_fee::text,
		       reconciled_at::text, reconciliation_notes, created_at::text
		FROM payments
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE
	`, tenantID, paymentID).Scan(
		&item.ID, &item.SaleID, &item.Method, &amount,
		&item.Provider, &item.TransactionRef, &item.AuthorizationCode, &item.Installments,
		&item.ReconciliationStatus, &reconciledAmount, &reconciledFee,
		&item.ReconciledAt, &item.ReconciliationNotes, &item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, common.ErrNotFound
		}
		return item, err
	}
	if item.Amount, err = platform.ParseMoney(amount); err != nil {
		return item, err
	}
	if reconciledAmount != nil {
		v, err := platform.ParseMoney(*reconciledAmount)
		if err != nil {
			return item, err
		}
		item.ReconciledAmount = &v
	}
	if reconciledFee != nil {
		v, err := platform.ParseMoney(*reconciledFee)
		if err != nil {
			return item, err
		}
		item.ReconciledFee = &v
	}
	return item, nil
}

func (r *FinanceRepo) CreatePaymentReconciliation(ctx context.Context, tx db.DBTX, tenantID string, item fin.PaymentReconciliation) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO payment_reconciliations(
			tenant_id, payment_id, expected_amount, received_amount, fee_amount,
			net_amount, difference_amount, status, provider, external_ref, notes, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text
	`, tenantID, item.PaymentID, item.ExpectedAmount.DBString(), item.ReceivedAmount.DBString(),
		item.FeeAmount.DBString(), item.NetAmount.DBString(), item.Difference.DBString(),
		item.Status, item.Provider, item.ExternalRef, item.Notes, item.CreatedBy).Scan(&id)
	return id, err
}

func (r *FinanceRepo) UpdatePaymentReconciliation(ctx context.Context, tx db.DBTX, tenantID, paymentID, status string, received, fee platform.Money, provider, externalRef, notes *string, actorUserID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE payments
		SET reconciliation_status=$3,
		    reconciled_amount=$4,
		    reconciled_fee=$5,
		    reconciled_at=now(),
		    reconciled_by_user_id=$6,
		    provider=COALESCE($7, provider),
		    transaction_ref=COALESCE($8, transaction_ref),
		    reconciliation_notes=$9
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, paymentID, status, received.DBString(), fee.DBString(), actorUserID, provider, externalRef, notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *FinanceRepo) ListReturnRefunds(ctx context.Context, tenantID, status string, limit, offset int) ([]fin.ReturnRefundSummary, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	statusClause := ""
	args := []any{tenantID}
	if status != "" {
		statusClause = " AND CASE WHEN COALESCE(rr.settled,0) >= sr.refund_due THEN 'settled' WHEN COALESCE(rr.settled,0) > 0 THEN 'partial' ELSE 'pending' END=$2"
		args = append(args, status)
	}
	countSQL := `
		WITH rr AS (
			SELECT tenant_id, return_id, SUM(amount) settled
			FROM return_refunds
			GROUP BY tenant_id, return_id
		)
		SELECT count(*)
		FROM sale_returns sr
		LEFT JOIN rr ON rr.tenant_id=sr.tenant_id AND rr.return_id=sr.id
		WHERE sr.tenant_id=$1` + statusClause
	var total int
	if err := r.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limitIdx := len(args)+1
	offsetIdx := len(args)+2
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		WITH rr AS (
			SELECT tenant_id, return_id, SUM(amount) settled
			FROM return_refunds
			GROUP BY tenant_id, return_id
		)
		SELECT sr.id::text, sr.sale_id::text, sr.kind, sr.reason,
		       sr.refund_due::text, COALESCE(rr.settled,0)::text,
		       (sr.refund_due-COALESCE(rr.settled,0))::text,
		       CASE WHEN COALESCE(rr.settled,0) >= sr.refund_due THEN 'settled'
		            WHEN COALESCE(rr.settled,0) > 0 THEN 'partial'
		            ELSE 'pending' END,
		       sr.created_at::text
		FROM sale_returns sr
		LEFT JOIN rr ON rr.tenant_id=sr.tenant_id AND rr.return_id=sr.id
		WHERE sr.tenant_id=$1%s
		ORDER BY sr.created_at DESC, sr.id DESC
		LIMIT $%d OFFSET $%d
	`, statusClause, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]fin.ReturnRefundSummary, 0)
	for rows.Next() {
		var item fin.ReturnRefundSummary
		var due, settled, remaining string
		if err := rows.Scan(&item.ReturnID, &item.SaleID, &item.Kind, &item.Reason, &due, &settled, &remaining, &item.Status, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		var err error
		if item.RefundDue, err = platform.ParseMoney(due); err != nil { return nil, 0, err }
		if item.SettledAmount, err = platform.ParseMoney(settled); err != nil { return nil, 0, err }
		if item.Remaining, err = platform.ParseMoney(remaining); err != nil { return nil, 0, err }
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *FinanceRepo) GetReturnForUpdate(ctx context.Context, tx db.DBTX, tenantID, returnID string) (fin.ReturnRefundSummary, error) {
	var item fin.ReturnRefundSummary
	var due string
	err := tx.QueryRow(ctx, `
		SELECT id::text, sale_id::text, kind, reason, refund_due::text, created_at::text
		FROM sale_returns
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE
	`, tenantID, returnID).Scan(&item.ReturnID, &item.SaleID, &item.Kind, &item.Reason, &due, &item.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return item, common.ErrNotFound
		}
		return item, err
	}
	item.RefundDue, err = platform.ParseMoney(due)
	return item, err
}

func (r *FinanceRepo) SumReturnRefunds(ctx context.Context, tx db.DBTX, tenantID, returnID string) (platform.Money, error) {
	var raw string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0)::text
		FROM return_refunds
		WHERE tenant_id=$1 AND return_id=$2
	`, tenantID, returnID).Scan(&raw); err != nil {
		return 0, err
	}
	return platform.ParseMoney(raw)
}

func (r *FinanceRepo) CreateReturnRefund(ctx context.Context, tx db.DBTX, tenantID string, item fin.ReturnRefund) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO return_refunds(
			tenant_id, return_id, sale_id, method, amount, provider, external_ref,
			cash_session_id, notes, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id::text
	`, tenantID, item.ReturnID, item.SaleID, item.Method, item.Amount.DBString(),
		item.Provider, item.ExternalRef, item.CashSessionID, item.Notes, item.CreatedBy).Scan(&id)
	return id, err
}

func (r *FinanceRepo) GetOpenCashAvailable(ctx context.Context, tx db.DBTX, tenantID, cashSessionID string) (platform.Money, error) {
	var openingRaw, cashSalesRaw, supplyRaw, withdrawalRaw string
	err := tx.QueryRow(ctx, `
		SELECT cs.opening_amount::text,
		       COALESCE((
		         SELECT SUM(p.amount)
		         FROM sales s
		         JOIN payments p ON p.sale_id=s.id AND p.tenant_id=s.tenant_id
		         WHERE s.tenant_id=cs.tenant_id AND s.cash_session_id=cs.id
		           AND s.status='finalized' AND p.method='cash'
		       ),0)::text,
		       COALESCE((
		         SELECT SUM(cm.amount) FROM cash_movements cm
		         WHERE cm.tenant_id=cs.tenant_id AND cm.cash_session_id=cs.id AND cm.movement_type='supply'
		       ),0)::text,
		       COALESCE((
		         SELECT SUM(cm.amount) FROM cash_movements cm
		         WHERE cm.tenant_id=cs.tenant_id AND cm.cash_session_id=cs.id AND cm.movement_type='withdrawal'
		       ),0)::text
		FROM cash_sessions cs
		WHERE cs.tenant_id=$1 AND cs.id=$2 AND cs.status='open'
		FOR UPDATE
	`, tenantID, cashSessionID).Scan(&openingRaw, &cashSalesRaw, &supplyRaw, &withdrawalRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, common.ErrCashSessionClosed
		}
		return 0, err
	}
	opening, err := platform.ParseMoney(openingRaw); if err != nil { return 0, err }
	cashSales, err := platform.ParseMoney(cashSalesRaw); if err != nil { return 0, err }
	supply, err := platform.ParseMoney(supplyRaw); if err != nil { return 0, err }
	withdrawal, err := platform.ParseMoney(withdrawalRaw); if err != nil { return 0, err }
	return opening.Add(cashSales).Add(supply).Sub(withdrawal), nil
}

func (r *FinanceRepo) InsertCashWithdrawal(ctx context.Context, tx db.DBTX, tenantID, cashSessionID, actorUserID string, amount platform.Money, notes *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO cash_movements(tenant_id, cash_session_id, movement_type, amount, notes, created_by_user_id)
		VALUES ($1,$2,'withdrawal',$3,$4,$5)
		RETURNING id::text
	`, tenantID, cashSessionID, amount.DBString(), notes, actorUserID).Scan(&id)
	return id, err
}

func (r *FinanceRepo) LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, tenantID+":"+operation+":"+key)
	return err
}

func (r *FinanceRepo) GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (resourceID, resultStatus, requestHash string, ok bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT resource_id::text, COALESCE(result_status,''), request_hash
		FROM finance_idempotency_keys
		WHERE tenant_id=$1 AND operation=$2 AND idem_key=$3
	`, tenantID, operation, key).Scan(&resourceID, &resultStatus, &requestHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", false, nil
		}
		return "", "", "", false, err
	}
	return resourceID, resultStatus, requestHash, true, nil
}

func (r *FinanceRepo) SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, resourceID, resultStatus string) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO finance_idempotency_keys(tenant_id, operation, idem_key, request_hash, resource_id, result_status)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, operation, idem_key) DO NOTHING
	`, tenantID, operation, key, requestHash, resourceID, resultStatus)
	if err == nil && tag.RowsAffected()==0 {
		return common.ErrConflict
	}
	return err
}
