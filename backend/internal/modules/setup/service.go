package setup

import (
	"context"
	"errors"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidStep = errors.New("invalid setup review step")

// Review is a manual acknowledgement, never evidence of stock reconciliation,
// employee provisioning, fiscal homologation or authorization to transmit.
type Review struct {
	Step       string    `json:"step"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

type Service struct {
	pool  *pgxpool.Pool
	audit *audit.Service
}

func New(pool *pgxpool.Pool, auditService *audit.Service) *Service {
	return &Service{pool: pool, audit: auditService}
}

func ValidStep(step string) bool {
	return step == "stock" || step == "team"
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Review, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT step, reviewed_at
		FROM setup_step_reviews
		WHERE tenant_id=$1
		ORDER BY step
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Review, 0, 2)
	for rows.Next() {
		var item Review
		if err := rows.Scan(&item.Step, &item.ReviewedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Set is tenant-scoped and audits in the same transaction; no notes or secrets are stored.
func (s *Service) Set(ctx context.Context, tenantID, actorID, step string, reviewed bool, requestID, ip, userAgent string) error {
	if !ValidStep(step) {
		return ErrInvalidStep
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if reviewed {
		_, err = tx.Exec(ctx, `
			INSERT INTO setup_step_reviews (tenant_id, step, reviewed_by_user_id)
			VALUES ($1,$2,$3)
			ON CONFLICT (tenant_id, step) DO UPDATE
			SET reviewed_by_user_id=EXCLUDED.reviewed_by_user_id,
				reviewed_at=now()
		`, tenantID, step, actorID)
	} else {
		_, err = tx.Exec(ctx, `
			DELETE FROM setup_step_reviews
			WHERE tenant_id=$1 AND step=$2
		`, tenantID, step)
	}
	if err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, db.DBTX(tx), audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorID,
		Action:       "setup.review.set",
		ResourceType: "setup_step_review",
		ResourceID:   tenantID,
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
		Metadata:     map[string]any{"step": step, "reviewed": reviewed},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
