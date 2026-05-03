package application

import (
	"context"

	authdomain "github.com/example/sistemaemgo/internal/modules/auth/domain"
)

// UsersRepository is an application-facing port.
// Infrastructure implements it (e.g. Postgres).
type UsersRepository interface {
	GetByEmail(ctx context.Context, email string) (authdomain.User, error)
	GetByID(ctx context.Context, id string) (authdomain.User, error)
	UpdateLastLogin(ctx context.Context, id string) error
	GetDefaultTenantID(ctx context.Context, userID string) (string, error)
	ListUserRoles(ctx context.Context, userID string, tenantID string) ([]string, error)
	ListUserPermissions(ctx context.Context, userID string, tenantID string) ([]string, error)
}

// RefreshTokenStore persists refresh token IDs (jti) to allow rotation and revocation.
// Implementations should guarantee that a refresh token can be consumed only once.
type RefreshTokenStore interface {
	Save(ctx context.Context, tokenID string, userID string, ttlSeconds int64) error
	Consume(ctx context.Context, tokenID string, userID string) (bool, error)
	Revoke(ctx context.Context, tokenID string) error
}
