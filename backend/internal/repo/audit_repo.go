package repo

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditRepo struct {
	db *pgxpool.Pool
}

func NewAuditRepo(db *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{db: db}
}

func (r *AuditRepo) Insert(ctx context.Context, tx DBTX, actorUserID *string, action, entityType string, entityID *string, metadataJSON string, ip *string, userAgent *string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_logs(actor_user_id, action, entity_type, entity_id, metadata, ip, user_agent)
		VALUES ($1,$2,$3,$4, COALESCE($5,'{}')::jsonb, $6::inet, $7)
	`, actorUserID, action, entityType, entityID, metadataJSON, ip, userAgent)
	return err
}
