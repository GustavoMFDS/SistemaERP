package infrastructure

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
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
	`, tenantID, e.EntryType, e.SaleID, e.CashSessionID, e.AmountGross, e.AmountDiscount, e.AmountNet, e.ProfitEstimated, e.Notes, createdByUserID).Scan(&id)
	return id, err
}

func (r *FinanceRepo) Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entry_type, COALESCE(SUM(amount_net),0)::float8
		FROM ledger_entries
		WHERE tenant_id=$1 AND created_at >= $2::timestamptz AND created_at < ($3::timestamptz + interval '1 day')
		GROUP BY entry_type
	`, tenantID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var t string
		var s float64
		if err := rows.Scan(&t, &s); err != nil {
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
		       amount_gross::float8, amount_discount::float8, amount_net::float8, profit_estimated::float8,
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
		if err := rows.Scan(&e.ID, &e.EntryType, &saleID, &cashID, &e.AmountGross, &e.AmountDiscount, &e.AmountNet, &e.ProfitEstimated, &e.Notes, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		e.SaleID = saleID
		e.CashSessionID = cashID
		items = append(items, e)
	}
	return items, total, rows.Err()
}
