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
	ListUserRoles(ctx context.Context, userID string) ([]string, error)
	ListUserPermissions(ctx context.Context, userID string) ([]string, error)
}
