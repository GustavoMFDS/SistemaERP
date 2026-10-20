//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Verify permissions after all migrations and the demo seed have run.
// Schema v34 may add permissions before the seed creates the manager role.
func TestDemoSeedCustomerAndTeamPermissions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := authinfra.NewUsersRepo(pool, false)
	testCases := []struct {
		email         string
		customerRead  bool
		customerWrite bool
		teamManage    bool
	}{
		{"admin@sistema.local", true, true, true},
		{"gerente@sistema.local", true, true, false},
		{"caixa@sistema.local", false, false, false},
	}
	for _, tc := range testCases {
		t.Run(tc.email, func(t *testing.T) {
			var userID, tenantID string
			err := pool.QueryRow(ctx, `
				SELECT u.id::text, ut.tenant_id::text
				FROM users u JOIN user_tenants ut ON ut.user_id=u.id
				JOIN companies c ON c.id=ut.tenant_id
				WHERE u.email=$1 AND c.cnpj='12345678000195'
			`, tc.email).Scan(&userID, &tenantID)
			if err != nil {
				t.Fatalf("expected seeded user and tenant: %v", err)
			}
			perms, err := repo.ListUserPermissions(ctx, userID, tenantID)
			if err != nil {
				t.Fatal(err)
			}
			set := make(map[string]bool, len(perms))
			for _, p := range perms {
				set[p] = true
			}
			for permission, expected := range map[string]bool{
				"customer:read":  tc.customerRead,
				"customer:write": tc.customerWrite,
				"team:manage":    tc.teamManage,
			} {
				if set[permission] != expected {
					t.Errorf("%s: permission %s expected %t, got %t", tc.email, permission, expected, set[permission])
				}
			}
		})
	}
}
