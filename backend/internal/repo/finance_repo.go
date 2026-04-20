package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FinanceRepo struct {
	db *pgxpool.Pool
}

type LedgerEntry struct {
	ID              string   `json:"id"`
	EntryType       string   `json:"entry_type"`
	SaleID          *string  `json:"sale_id"`
	CashSessionID   *string  `json:"cash_session_id"`
	AmountGross     float64  `json:"amount_gross"`
	AmountDiscount  float64  `json:"amount_discount"`
	AmountNet       float64  `json:"amount_net"`
	ProfitEstimated float64  `json:"profit_estimated"`
	Notes           *string  `json:"notes"`
	CreatedAt       string   `json:"created_at"`
}

func NewFinanceRepo(db *pgxpool.Pool) *FinanceRepo {
	return &FinanceRepo{db: db}
}

func (r *FinanceRepo) InsertLedgerEntry(ctx context.Context, tx DBTX, e LedgerEntry, createdByUserID *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_entries(entry_type, sale_id, cash_session_id, amount_gross, amount_discount, amount_net, profit_estimated, notes, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id::text
	`, e.EntryType, e.SaleID, e.CashSessionID, e.AmountGross, e.AmountDiscount, e.AmountNet, e.ProfitEstimated, e.Notes, createdByUserID).Scan(&id)
	return id, err
}

func (r *FinanceRepo) Dashboard(ctx context.Context, from, to string) (map[string]float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entry_type, COALESCE(SUM(amount_net),0)::float8
		FROM ledger_entries
		WHERE created_at >= $1::timestamptz AND created_at < ($2::timestamptz + interval '1 day')
		GROUP BY entry_type
	`, from, to)
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

func (r *FinanceRepo) ListLedger(ctx context.Context, limit, offset int) ([]LedgerEntry, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM ledger_entries`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, entry_type, sale_id::text, cash_session_id::text,
		       amount_gross::float8, amount_discount::float8, amount_net::float8, profit_estimated::float8,
		       notes, created_at::text
		FROM ledger_entries
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
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
