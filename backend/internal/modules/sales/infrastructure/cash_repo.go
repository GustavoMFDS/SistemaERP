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
	// Keep first-use registration safe when two terminals open the PDV at the
	// same time. The insert and select are separate statements so a waiter that
	// loses the unique-key race gets a fresh READ COMMITTED snapshot and can see
	// the register committed by the winner.
	if _, err := r.db.Exec(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		SELECT $1::uuid, 'Caixa Principal', true
		WHERE NOT EXISTS (SELECT 1 FROM cash_registers WHERE tenant_id=$1)
		ON CONFLICT (tenant_id, name) DO NOTHING
	`, tenantID); err != nil {
		return "", err
	}

	var id string
	err := r.db.QueryRow(ctx, `
		SELECT id::text
		FROM cash_registers
		WHERE tenant_id=$1
		ORDER BY created_at, id
		LIMIT 1
	`, tenantID).Scan(&id)
	return id, err
}

func (r *CashRepo) GetOpenSession(ctx context.Context, tenantID string) (sales.CashSession, error) {
	var s sales.CashSession
	var openingRaw string
	err := r.db.QueryRow(ctx, `
		SELECT cs.id::text, cs.cash_register_id::text, cs.opened_by_user_id::text, cs.status, cs.opening_amount::text
		FROM cash_sessions cs
		WHERE cs.tenant_id=$1
		  AND cs.status='open'
		  AND cs.cash_register_id = (
			SELECT cr.id
			FROM cash_registers cr
			WHERE cr.tenant_id=$1
			ORDER BY cr.created_at, cr.id
			LIMIT 1
		  )
		ORDER BY cs.opened_at DESC
		LIMIT 1
	`, tenantID).Scan(&s.ID, &s.RegisterID, &s.OpenedByUserID, &s.Status, &openingRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s, common.ErrNotFound
		}
		return s, err
	}
	opening, err := platform.ParseMoney(openingRaw)
	if err != nil {
		return s, err
	}
	s.OpeningAmount = opening
	return s, nil
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
		WITH method_totals AS (
			SELECT p.method, p.amount
			FROM sales s
			JOIN payments p ON p.sale_id=s.id AND p.tenant_id=s.tenant_id
			WHERE s.tenant_id=$1
			  AND s.cash_session_id=$2
			  AND s.status='finalized'
			UNION ALL
			SELECT rr.method, -rr.amount
			FROM return_refunds rr
			WHERE rr.tenant_id=$1
			  AND rr.cash_session_id=$2
			  AND rr.method <> 'cash'
		)
		SELECT method, COALESCE(SUM(amount),0)::text
		FROM method_totals
		GROUP BY method
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
		difference, err := dec.SubChecked(exp)
		if err != nil {
			return common.ErrValidation
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cash_session_reconciliations(
				tenant_id, cash_session_id, method, expected_amount, declared_amount, difference_amount
			)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, tenantID, sessionID, method, exp.DBString(), dec.DBString(), difference.DBString()); err != nil {
			return err
		}
	}
	return nil
}
