package bootstrapadmin

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status is evidence about the first-admin transaction, never permission to
// retry bootstrap, enable services, or resume an installation.
type Status string

const (
	AdminAbsent        Status = "ADMIN_ABSENT"
	AdminCommitted     Status = "ADMIN_COMMITTED"
	AdminIndeterminate Status = "ADMIN_INDETERMINATE"
)

type statusCounts struct {
	companies, placeholders, users, memberships, roles, tenantRoles, auditEvents, matching int
}

func classifyStatus(c statusCounts) Status {
	if c.companies == 1 && c.placeholders == 1 && c.users == 0 &&
		c.memberships == 0 && c.roles == 0 && c.tenantRoles == 0 && c.auditEvents == 0 && c.matching == 0 {
		return AdminAbsent
	}
	if c.companies == 1 && c.placeholders == 0 && c.users == 1 &&
		c.memberships == 1 && c.roles == 1 && c.tenantRoles == 1 && c.auditEvents == 1 && c.matching == 1 {
		return AdminCommitted
	}
	return AdminIndeterminate
}

// Inspect reads a consistent snapshot through the restricted application role.
// It returns only fixed status labels. No identities, hashes or DB errors leave
// this boundary. It neither verifies the password nor proves a pristine schema.
func Inspect(ctx context.Context, pool *pgxpool.Pool) (Status, error) {
	if pool == nil {
		return AdminIndeterminate, errors.New("database pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return AdminIndeterminate, errors.New("could not begin read-only admin inspection")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var db string
	var privileged bool
	if err := tx.QueryRow(ctx, `
		SELECT current_database(), rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication OR rolbypassrls
		FROM pg_roles WHERE rolname=current_user
	`).Scan(&db, &privileged); err != nil || privileged || db == "postgres" || strings.HasPrefix(db, "template") {
		return AdminIndeterminate, errors.New("admin inspection requires the restricted application database role")
	}
	var c statusCounts
	err = tx.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM companies),
		  (SELECT count(*) FROM companies WHERE cnpj=$1),
		  (SELECT count(*) FROM users),
		  (SELECT count(*) FROM user_tenants),
		  (SELECT count(*) FROM user_roles),
		  (SELECT count(*) FROM user_tenant_roles),
		  (SELECT count(*) FROM audit_logs WHERE action='install.first_admin.create'),
		  (SELECT count(*) FROM users u
		    JOIN user_tenants ut ON ut.user_id=u.id
		    JOIN companies co ON co.id=ut.tenant_id
		    JOIN user_roles ur ON ur.user_id=u.id
		    JOIN roles r ON r.id=ur.role_id AND r.name='admin'
		    JOIN user_tenant_roles utr ON utr.user_id=u.id AND utr.tenant_id=co.id AND utr.role_id=r.id
		    JOIN audit_logs a ON a.actor_user_id=u.id AND a.tenant_id=co.id AND a.resource_id=u.id
		      AND a.action='install.first_admin.create' AND a.resource_type='user'
		    WHERE u.active AND co.cnpj<>$1
		      AND EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id=r.id))
	`, placeholderCNPJ).Scan(&c.companies, &c.placeholders, &c.users, &c.memberships, &c.roles, &c.tenantRoles, &c.auditEvents, &c.matching)
	if err != nil {
		return AdminIndeterminate, errors.New("could not inspect first-admin state")
	}
	return classifyStatus(c), nil
}
