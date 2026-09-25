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

func (r *CashRepo) GetOpenSession(ctx context.Context, tenantID string) (sales.CashSession, bool, error) {
	var s sales.CashSession
	var openingRaw string
	err := r.db.QueryRow(ctx, `
		WITH default_register AS (
			SELECT id
			FROM cash_registers
			WHERE tenant_id=$1
			ORDER BY created_at
			LIMIT 1
		)
		SELECT cs.id::text, cs.cash_register_id::text, cs.opened_by_user_id::text, cs.status, cs.opening_amount::text
		FROM default_register dr
		JOIN cash_sessions cs
		  ON cs.cash_register_id=dr.id
		 AND cs.tenant_id=$1
		WHERE cs.status='open'
		ORDER BY cs.opened_at DESC
		LIMIT 1
	`, tenantID).Scan(&s.ID, &s.RegisterID, &s.OpenedByUserID, &s.Status, &openingRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return sales.CashSession{}, false, nil
	}
	if err != nil {
		return sales.CashSession{}, false, err
	}
	opening, err := platform.ParseMoney(openingRaw)
	if err != nil {
		return sales.CashSession{}, false, err
	}
	s.OpeningAmount = opening
	return s, true, nil
}

func (r *CashRepo) CloseSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID, userID string, expectedCash, closingAmount platform.Money, notes *string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE cash_sessions
		SET status='closed',
		    closed_at=now(),
		    closed_by_user_id=$2,
		    closing_amount=$3,
		    expected_cash=$4,
		    closing_difference=$3::numeric - $4::numeric,
		    notes=COALESCE($5, notes)
		WHERE tenant_id=$6 AND id=$1 AND status='open'
	`, sessionID, userID, closingAmount.DBString(), expectedCash.DBString(), notes, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrCashSessionClosed
	}
	return nil
}

func (r *CashRepo) GetSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID string) (sales.CashSession, error) {
	var s sales.CashSession
	var openingRaw string
	err := tx.QueryRow(ctx, `
		SELECT id::text, cash_register_id::text, opened_by_user_id::text, status, opening_amount::text
		FROM cash_sessions
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE
	`, tenantID, sessionID).
		Scan(&s.ID, &s.RegisterID, &s.OpenedByUserID, &s.Status, &openingRaw)
	if err != nil {
		return s, err
	}
	opening, err := platform.ParseMoney(openingRaw)
	if err != nil {
		return s, err
	}
	s.OpeningAmount = opening
	return s, nil
}

func (r *CashRepo) InsertMovement(ctx context.Context, tx db.DBTX, tenantID, sessionID, userID, movementType string, amount platform.Money, notes *string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO cash_movements(
			tenant_id, cash_session_id, movement_type, amount, notes, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id::text
	`, tenantID, sessionID, movementType, amount.DBString(), notes, userID).Scan(&id)
	return id, err
}

func (r *CashRepo) SumPaymentsByMethod(ctx context.Context, tx db.DBTX, tenantID, sessionID string) (map[string]platform.Money, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.method, COALESCE(SUM(p.amount),0)::text
		FROM sales s
		JOIN payments p ON p.sale_id=s.id AND p.tenant_id=s.tenant_id
		WHERE s.tenant_id=$1
		  AND s.cash_session_id=$2
		  AND s.status='finalized'
		GROUP BY p.method
	`, tenantID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]platform.Money{}
	for rows.Next() {
		var method, raw string
		if err := rows.Scan(&method, &raw); err != nil {
			return nil, err
		}
		amount, err := platform.ParseMoney(raw)
		if err != nil {
			return nil, err
		}
		out[method] = amount
	}
	return out, rows.Err()
}

func (r *CashRepo) SumMovements(ctx context.Context, tx db.DBTX, tenantID, sessionID string) (platform.Money, platform.Money, error) {
	var supplyRaw, withdrawalRaw string
	err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN movement_type='supply' THEN amount ELSE 0 END),0)::text,
			COALESCE(SUM(CASE WHEN movement_type='withdrawal' THEN amount ELSE 0 END),0)::text
		FROM cash_movements
		WHERE tenant_id=$1 AND cash_session_id=$2
	`, tenantID, sessionID).Scan(&supplyRaw, &withdrawalRaw)
	if err != nil {
		return 0, 0, err
	}
	supply, err := platform.ParseMoney(supplyRaw)
	if err != nil {
		return 0, 0, err
	}
	withdrawal, err := platform.ParseMoney(withdrawalRaw)
	if err != nil {
		return 0, 0, err
	}
	return supply, withdrawal, nil
}

func (r *CashRepo) SaveReconciliation(ctx context.Context, tx db.DBTX, tenantID, sessionID string, expected, declared map[string]platform.Money) error {
	methods := []string{"cash", "pix", "debit", "credit", "transfer", "voucher"}
	for _, method := range methods {
		exp := expected[method]
		dec := declared[method]
		if _, err := tx.Exec(ctx, `
			INSERT INTO cash_session_reconciliations(
				tenant_id, cash_session_id, method, expected_amount, declared_amount, difference_amount
			)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, tenantID, sessionID, method, exp.DBString(), dec.DBString(), dec.Sub(exp).DBString()); err != nil {
			return err
		}
	}
	return nil
}
