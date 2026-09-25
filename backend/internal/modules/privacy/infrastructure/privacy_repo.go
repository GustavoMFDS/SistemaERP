package infrastructure

import (
	"context"
	"fmt"

	"github.com/example/sistemaemgo/internal/modules/common"
	privacyapp "github.com/example/sistemaemgo/internal/modules/privacy/application"
	privacy "github.com/example/sistemaemgo/internal/modules/privacy/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(dbpool *pgxpool.Pool) *Repo {
	return &Repo{db: dbpool}
}

func (r *Repo) SubjectBelongsToTenant(ctx context.Context, tenantID, subjectType, subjectID string) (bool, error) {
	var exists bool
	switch subjectType {
	case "customer":
		err := r.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM customers
				WHERE tenant_id=$1 AND id=$2
			)
		`, tenantID, subjectID).Scan(&exists)
		return exists, err
	case "user":
		err := r.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM user_tenants
				WHERE tenant_id=$1 AND user_id=$2
			)
		`, tenantID, subjectID).Scan(&exists)
		return exists, err
	default:
		return false, fmt.Errorf("unsupported subject type")
	}
}

func (r *Repo) CreateRequest(ctx context.Context, tenantID, actorUserID, requestID string, req privacyapp.CreateRequest) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO data_subject_requests(
			tenant_id, subject_type, subject_id, requester_email, request_type,
			notes, created_by_user_id, request_id
		)
		VALUES ($1,$2,NULLIF($3, '')::uuid,$4,$5,$6,NULLIF($7, '')::uuid,$8)
		RETURNING id::text
	`, tenantID, req.SubjectType, strPtr(req.SubjectID), req.RequesterEmail, req.RequestType, req.Notes, actorUserID, requestID).Scan(&id)
	return id, err
}

func (r *Repo) ListRequests(ctx context.Context, tenantID string, limit, offset int) ([]privacy.DataSubjectRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, tenant_id::text, subject_type, subject_id::text, requester_email::text,
		       request_type, status, notes, requested_at::text, resolved_at::text,
		       created_by_user_id::text, request_id
		FROM data_subject_requests
		WHERE tenant_id=$1
		ORDER BY requested_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

func (r *Repo) GetRequest(ctx context.Context, tenantID, id string) (privacy.DataSubjectRequest, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, tenant_id::text, subject_type, subject_id::text, requester_email::text,
		       request_type, status, notes, requested_at::text, resolved_at::text,
		       created_by_user_id::text, request_id
		FROM data_subject_requests
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	if err != nil {
		return privacy.DataSubjectRequest{}, err
	}
	defer rows.Close()
	items, err := scanRequests(rows)
	if err != nil {
		return privacy.DataSubjectRequest{}, err
	}
	if len(items) == 0 {
		return privacy.DataSubjectRequest{}, pgx.ErrNoRows
	}
	return items[0], nil
}

func (r *Repo) UpdateRequestStatus(ctx context.Context, tenantID, id, status string, notes *string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE data_subject_requests
		SET status=$3,
		    notes=COALESCE($4, notes),
		    resolved_at=CASE WHEN $3 IN ('completed', 'rejected', 'cancelled') THEN now() ELSE resolved_at END
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status, notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repo) ExportSubjectData(ctx context.Context, tenantID, subjectType, subjectID string) (map[string]any, error) {
	switch subjectType {
	case "customer":
		return r.exportCustomer(ctx, tenantID, subjectID)
	case "user":
		return r.exportUser(ctx, tenantID, subjectID)
	default:
		return nil, fmt.Errorf("unsupported subject type")
	}
}

func (r *Repo) AnonymizeSubject(ctx context.Context, tenantID, subjectType, subjectID string) error {
	switch subjectType {
	case "customer":
		tag, err := r.db.Exec(ctx, `
			UPDATE customers
			SET name='Titular anonimizado', document=NULL, email=NULL, phone=NULL, updated_at=now()
			WHERE tenant_id=$1 AND id=$2
		`, tenantID, subjectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	case "user":
		tx, err := r.db.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var lockedUserID string
		if err := tx.QueryRow(ctx, `
			SELECT u.id::text
			FROM users u
			JOIN user_tenants ut ON ut.user_id=u.id
			WHERE ut.tenant_id=$1 AND u.id=$2
			FOR UPDATE OF u
		`, tenantID, subjectID).Scan(&lockedUserID); err != nil {
			return err
		}

		var membershipCount int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM user_tenants
			WHERE user_id=$1
		`, subjectID).Scan(&membershipCount); err != nil {
			return err
		}
		if membershipCount != 1 {
			return common.ErrConflict
		}

		tag, err := tx.Exec(ctx, `
			UPDATE users
			SET name='Usuario anonimizado',
			    email=('anon+' || id::text || '@anonymized.local')::citext,
			    active=false,
			    updated_at=now()
			WHERE id=$1
		`, subjectID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}

		if _, err := tx.Exec(ctx, `
			UPDATE user_tenants
			SET active=false
			WHERE tenant_id=$1 AND user_id=$2
		`, tenantID, subjectID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	default:
		return fmt.Errorf("unsupported subject type")
	}
}

func (r *Repo) BlockSubject(ctx context.Context, tenantID, subjectType, subjectID string) error {
	if subjectType != "user" {
		return fmt.Errorf("blocking is only supported for users")
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE user_tenants
		SET active=false
		WHERE tenant_id=$1 AND user_id=$2
	`, tenantID, subjectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repo) RecordConsent(ctx context.Context, tenantID, requestID string, req privacyapp.ConsentCreateRequest) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO consent_records(
			tenant_id, subject_type, subject_id, purpose, consent_text_version, source, request_id
		)
		VALUES ($1,$2,NULLIF($3, '')::uuid,$4,$5,$6,$7)
		RETURNING id::text
	`, tenantID, req.SubjectType, strPtr(req.SubjectID), req.Purpose, req.ConsentTextVersion, req.Source, requestID).Scan(&id)
	return id, err
}

func (r *Repo) ListConsents(ctx context.Context, tenantID string, limit, offset int) ([]privacy.ConsentRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, tenant_id::text, subject_type, subject_id::text, purpose,
		       consent_text_version, consented_at::text, source, withdrawn_at::text, request_id
		FROM consent_records
		WHERE tenant_id=$1
		ORDER BY consented_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []privacy.ConsentRecord
	for rows.Next() {
		var item privacy.ConsentRecord
		if err := rows.Scan(&item.ID, &item.TenantID, &item.SubjectType, &item.SubjectID, &item.Purpose, &item.ConsentTextVersion, &item.ConsentedAt, &item.Source, &item.WithdrawnAt, &item.RequestID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) RevokeConsent(ctx context.Context, tenantID, id string) error {
	tag, err := r.db.Exec(ctx, `UPDATE consent_records SET withdrawn_at=COALESCE(withdrawn_at, now()) WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repo) exportCustomer(ctx context.Context, tenantID, subjectID string) (map[string]any, error) {
	var id, name string
	var document, email, phone *string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, name, document, email::text, phone
		FROM customers
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, subjectID).Scan(&id, &name, &document, &email, &phone)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"subject_type": "customer",
		"id":           id,
		"name":         name,
		"document":     document,
		"email":        email,
		"phone":        phone,
		"sales":        r.customerSales(ctx, tenantID, subjectID),
		"requests":     r.subjectRequests(ctx, tenantID, "customer", subjectID),
		"consents":     r.subjectConsents(ctx, tenantID, "customer", subjectID),
	}, nil
}

func (r *Repo) exportUser(ctx context.Context, tenantID, subjectID string) (map[string]any, error) {
	var id, email, name string
	var active bool
	var lastLoginAt *string
	err := r.db.QueryRow(ctx, `
		SELECT u.id::text, u.email::text, u.name, u.active, u.last_login_at::text
		FROM users u
		JOIN user_tenants ut ON ut.user_id=u.id
		WHERE ut.tenant_id=$1 AND u.id=$2
	`, tenantID, subjectID).Scan(&id, &email, &name, &active, &lastLoginAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"subject_type":  "user",
		"id":            id,
		"email":         email,
		"name":          name,
		"active":        active,
		"last_login_at": lastLoginAt,
		"requests":      r.subjectRequests(ctx, tenantID, "user", subjectID),
		"consents":      r.subjectConsents(ctx, tenantID, "user", subjectID),
		"audit_events":  r.actorAuditEvents(ctx, tenantID, subjectID),
	}, nil
}

func (r *Repo) customerSales(ctx context.Context, tenantID, subjectID string) []map[string]any {
	rows, err := r.db.Query(ctx, `
		SELECT s.id::text, s.status, s.total::text, s.finalized_at::text, i.id::text
		FROM sales s
		LEFT JOIN invoices i ON i.sale_id=s.id AND i.tenant_id=s.tenant_id
		WHERE s.tenant_id=$1 AND s.customer_id=$2
		ORDER BY s.finalized_at DESC
		LIMIT 200
	`, tenantID, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, status, total, finalizedAt string
		var invoiceID *string
		if err := rows.Scan(&id, &status, &total, &finalizedAt, &invoiceID); err != nil {
			return out
		}
		out = append(out, map[string]any{"id": id, "status": status, "total": total, "finalized_at": finalizedAt, "invoice_id": invoiceID})
	}
	return out
}

func (r *Repo) subjectRequests(ctx context.Context, tenantID, subjectType, subjectID string) []map[string]any {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, request_type, status, requested_at::text, resolved_at::text
		FROM data_subject_requests
		WHERE tenant_id=$1 AND subject_type=$2 AND subject_id=$3
		ORDER BY requested_at DESC
		LIMIT 200
	`, tenantID, subjectType, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, requestType, status, requestedAt string
		var resolvedAt *string
		if err := rows.Scan(&id, &requestType, &status, &requestedAt, &resolvedAt); err != nil {
			return out
		}
		out = append(out, map[string]any{"id": id, "request_type": requestType, "status": status, "requested_at": requestedAt, "resolved_at": resolvedAt})
	}
	return out
}

func (r *Repo) subjectConsents(ctx context.Context, tenantID, subjectType, subjectID string) []map[string]any {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, purpose, consent_text_version, consented_at::text, source, withdrawn_at::text
		FROM consent_records
		WHERE tenant_id=$1 AND subject_type=$2 AND subject_id=$3
		ORDER BY consented_at DESC
		LIMIT 200
	`, tenantID, subjectType, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, purpose, version, consentedAt, source string
		var withdrawnAt *string
		if err := rows.Scan(&id, &purpose, &version, &consentedAt, &source, &withdrawnAt); err != nil {
			return out
		}
		out = append(out, map[string]any{"id": id, "purpose": purpose, "consent_text_version": version, "consented_at": consentedAt, "source": source, "withdrawn_at": withdrawnAt})
	}
	return out
}

func (r *Repo) actorAuditEvents(ctx context.Context, tenantID, subjectID string) []map[string]any {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, action, resource_type, resource_id::text, request_id, created_at::text
		FROM audit_logs
		WHERE tenant_id=$1 AND actor_user_id=$2
		ORDER BY created_at DESC
		LIMIT 200
	`, tenantID, subjectID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, action, resourceType, createdAt string
		var resourceID, requestID *string
		if err := rows.Scan(&id, &action, &resourceType, &resourceID, &requestID, &createdAt); err != nil {
			return out
		}
		out = append(out, map[string]any{"id": id, "action": action, "resource_type": resourceType, "resource_id": resourceID, "request_id": requestID, "created_at": createdAt})
	}
	return out
}

func scanRequests(rows pgx.Rows) ([]privacy.DataSubjectRequest, error) {
	var items []privacy.DataSubjectRequest
	for rows.Next() {
		var item privacy.DataSubjectRequest
		if err := rows.Scan(&item.ID, &item.TenantID, &item.SubjectType, &item.SubjectID, &item.RequesterEmail, &item.RequestType, &item.Status, &item.Notes, &item.RequestedAt, &item.ResolvedAt, &item.CreatedByUserID, &item.RequestID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func strPtr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
