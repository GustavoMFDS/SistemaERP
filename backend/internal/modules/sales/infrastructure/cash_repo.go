package infrastructure

import (
	"context"
	"errors"

	"github.com/example/sistemaemgo/internal/modules/common"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CashRepo struct {
	db *pgxpool.Pool
}

func NewCashRepo(dbpool *pgxpool.Pool) *CashRepo {
	return &CashRepo{db: dbpool}
}

func (r *CashRepo) EnsureDefaultRegister(ctx context.Context, tenantID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO cash_registers(tenant_id, name, active)
			SELECT $1::uuid, 'Caixa Principal', true
			WHERE NOT EXISTS (SELECT 1 FROM cash_registers WHERE tenant_id=$1)
			RETURNING id, created_at
		)
		SELECT id::text
		FROM (
			SELECT id, created_at FROM ins
			UNION ALL
			SELECT id, created_at FROM cash_registers WHERE tenant_id=$1
		) registers
		ORDER BY created_at
		LIMIT 1
	`, tenantID).Scan(&id)
	return id, err
}

func (r *CashRepo) OpenSession(ctx context.Context, tx db.DBTX, tenantID string, registerID, userID string, openingAmount platform.Money, notes *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO cash_sessions(tenant_id, cash_register_id, opened_by_user_id, opening_amount, notes)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text
	`, tenantID, registerID, userID, openingAmount.DBString(), notes).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "cash_sessions_one_open_per_register" {
			return "", common.ErrCashSessionAlreadyOpen
		}
	}
	return id, err
}

func (r *CashRepo) CloseSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID, userID string, closingAmount platform.Money, notes *string) (sales.CashCloseResult, error) {
	var expectedRaw, closingRaw, differenceRaw string
	err := tx.QueryRow(ctx, `
		WITH expected AS (
			SELECT
				cs.id,
				cs.opening_amount +
				COALESCE(SUM(CASE
					WHEN p.method='cash' AND s.status='finalized' THEN p.amount
					ELSE 0
				END), 0) AS expected_cash
			FROM cash_sessions cs
			LEFT JOIN sales s
				ON s.cash_session_id=cs.id
				AND s.tenant_id=cs.tenant_id
			LEFT JOIN payments p ON p.sale_id=s.id
			WHERE cs.tenant_id=$5 AND cs.id=$1 AND cs.status='open'
			GROUP BY cs.id, cs.opening_amount
		)
		UPDATE cash_sessions cs
		SET status='closed',
		    closed_at=now(),
		    closed_by_user_id=$2,
		    closing_amount=$3,
		    expected_cash=expected.expected_cash,
		    closing_difference=$3::numeric - expected.expected_cash,
		    notes=COALESCE($4, cs.notes)
		FROM expected
		WHERE cs.id=expected.id
		RETURNING cs.expected_cash::text, cs.closing_amount::text, cs.closing_difference::text
	`, sessionID, userID, closingAmount.DBString(), notes, tenantID).
		Scan(&expectedRaw, &closingRaw, &differenceRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sales.CashCloseResult{}, common.ErrCashSessionClosed
		}
		return sales.CashCloseResult{}, err
	}
	expected, err := platform.ParseMoney(expectedRaw)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	closing, err := platform.ParseMoney(closingRaw)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	difference, err := platform.ParseMoney(differenceRaw)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	return sales.CashCloseResult{
		ExpectedCash:      expected,
		ClosingAmount:     closing,
		ClosingDifference: difference,
	}, nil
}

func (r *CashRepo) GetSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID string) (sales.CashSession, error) {
	var s sales.CashSession
	err := tx.QueryRow(ctx, `
		SELECT id::text, cash_register_id::text, opened_by_user_id::text, status
		FROM cash_sessions
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE
	`, tenantID, sessionID).
		Scan(&s.ID, &s.RegisterID, &s.OpenedByUserID, &s.Status)
	return s, err
}
