package infrastructure

import (
	"context"
	"errors"

	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	authdomain "github.com/example/sistemaemgo/internal/modules/auth/domain"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsersRepo struct {
	db                  *pgxpool.Pool
	allowTenantFallback bool
}

func NewUsersRepo(db *pgxpool.Pool, allowTenantFallback bool) *UsersRepo {
	return &UsersRepo{db: db, allowTenantFallback: allowTenantFallback}
}

func (r *UsersRepo) GetByEmail(ctx context.Context, email string) (authdomain.User, error) {
	var u authdomain.User
	err := r.db.QueryRow(ctx, `SELECT id::text, email::text, name, password_hash, active FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Active)
	return u, err
}

func (r *UsersRepo) GetByID(ctx context.Context, id string) (authdomain.User, error) {
	var u authdomain.User
	err := r.db.QueryRow(ctx, `SELECT id::text, email::text, name, password_hash, active FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Active)
	return u, err
}

func (r *UsersRepo) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET last_login_at=now(), updated_at=now() WHERE id=$1`, id)
	return err
}

func (r *UsersRepo) GetDefaultTenantID(ctx context.Context, userID string) (string, error) {
	var tenantID string
	err := r.db.QueryRow(ctx, `
		SELECT tenant_id::text
		FROM user_tenants
		WHERE user_id=$1 AND active=true
		ORDER BY created_at
		LIMIT 1
	`, userID).Scan(&tenantID)
	if err == nil {
		return tenantID, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if !r.allowTenantFallback {
			return "", common.ErrForbidden
		}
		var hasAnyTenant bool
		if err := r.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM user_tenants WHERE user_id=$1)
		`, userID).Scan(&hasAnyTenant); err != nil {
			return "", err
		}
		if hasAnyTenant {
			// An explicit (but inactive) membership must never fall through to
			// the legacy first-company development fallback.
			return "", common.ErrForbidden
		}
		// Legacy development/test fallback for databases/users that truly
		// predate tenant membership data.
		return r.fallbackCompanyTenantID(ctx)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		if !r.allowTenantFallback {
			return "", common.ErrForbidden
		}
		// Legacy development/test fallback for databases that have not applied the tenant migration yet.
		return r.fallbackCompanyTenantID(ctx)
	}
	return "", err
}

func (r *UsersRepo) fallbackCompanyTenantID(ctx context.Context) (string, error) {
	var tenantID string
	err := r.db.QueryRow(ctx, `SELECT id::text FROM companies ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	return tenantID, err
}

func (r *UsersRepo) ListUserTenants(ctx context.Context, userID string) ([]authapp.AuthTenantInfo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id::text, c.legal_name, c.trade_name
		FROM user_tenants ut
		JOIN companies c ON c.id=ut.tenant_id
		WHERE ut.user_id=$1 AND ut.active=true
		ORDER BY COALESCE(c.trade_name, c.legal_name), c.legal_name, c.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []authapp.AuthTenantInfo
	for rows.Next() {
		var item authapp.AuthTenantInfo
		if err := rows.Scan(&item.ID, &item.LegalName, &item.TradeName); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *UsersRepo) UserHasTenant(ctx context.Context, userID string, tenantID string) (bool, error) {
	var hasRequestedTenant bool
	var hasAnyTenant bool
	err := r.db.QueryRow(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM user_tenants WHERE user_id=$1 AND tenant_id=$2 AND active=true),
			EXISTS(SELECT 1 FROM user_tenants WHERE user_id=$1)
	`, userID, tenantID).Scan(&hasRequestedTenant, &hasAnyTenant)
	if err == nil {
		if hasRequestedTenant {
			return true, nil
		}
		if !r.allowTenantFallback || hasAnyTenant {
			return false, nil
		}
		fallbackID, fallbackErr := r.fallbackCompanyTenantID(ctx)
		if fallbackErr != nil {
			return false, fallbackErr
		}
		return fallbackID == tenantID, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" && r.allowTenantFallback {
		fallbackID, fallbackErr := r.fallbackCompanyTenantID(ctx)
		if fallbackErr != nil {
			return false, fallbackErr
		}
		return fallbackID == tenantID, nil
	}
	return false, err
}

func (r *UsersRepo) ListUserRoles(ctx context.Context, userID string, tenantID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.name
		FROM user_tenant_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN user_tenants ut ON ut.user_id=ur.user_id AND ut.tenant_id=ur.tenant_id AND ut.active=true
		WHERE ur.user_id=$1 AND ur.tenant_id=$2
		ORDER BY r.name
	`, userID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

func (r *UsersRepo) ListUserPermissions(ctx context.Context, userID string, tenantID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT p.code
		FROM user_tenant_roles ur
		JOIN user_tenants ut ON ut.user_id=ur.user_id AND ut.tenant_id=ur.tenant_id AND ut.active=true
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id=$1 AND ur.tenant_id=$2
		ORDER BY p.code
	`, userID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		perms = append(perms, code)
	}
	return perms, rows.Err()
}
