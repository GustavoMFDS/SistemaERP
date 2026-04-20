package infrastructure

import (
	"context"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CashRepo struct {
	db *pgxpool.Pool
}

func NewCashRepo(dbpool *pgxpool.Pool) *CashRepo {
	return &CashRepo{db: dbpool}
}

func (r *CashRepo) EnsureDefaultRegister(ctx context.Context) (string, error) {
	// create a default register if none exists
	var id string
	err := r.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO cash_registers(name, active)
			SELECT 'Caixa Principal', true
			WHERE NOT EXISTS (SELECT 1 FROM cash_registers)
			RETURNING id
		)
		SELECT id::text FROM cash_registers ORDER BY created_at LIMIT 1
	`).Scan(&id)
	return id, err
}

func (r *CashRepo) OpenSession(ctx context.Context, tx db.DBTX, registerID, userID string, openingAmount float64, notes *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO cash_sessions(cash_register_id, opened_by_user_id, opening_amount, notes)
		VALUES ($1,$2,$3,$4)
		RETURNING id::text
	`, registerID, userID, openingAmount, notes).Scan(&id)
	return id, err
}

func (r *CashRepo) CloseSession(ctx context.Context, tx db.DBTX, sessionID, userID string, closingAmount float64, notes *string) error {
	_, err := tx.Exec(ctx, `
		UPDATE cash_sessions
		SET status='closed', closed_at=now(), closed_by_user_id=$2, closing_amount=$3, notes=COALESCE($4, notes)
		WHERE id=$1 AND status='open'
	`, sessionID, userID, closingAmount, notes)
	return err
}

func (r *CashRepo) GetSession(ctx context.Context, tx db.DBTX, sessionID string) (sales.CashSession, error) {
	var s sales.CashSession
	err := tx.QueryRow(ctx, `SELECT id::text, cash_register_id::text, opened_by_user_id::text, status FROM cash_sessions WHERE id=$1`, sessionID).
		Scan(&s.ID, &s.RegisterID, &s.OpenedByUserID, &s.Status)
	return s, err
}
