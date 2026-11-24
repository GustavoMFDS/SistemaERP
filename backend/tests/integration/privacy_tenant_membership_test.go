//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/example/sistemaemgo/internal/modules/common"
	privacyinfra "github.com/example/sistemaemgo/internal/modules/privacy/infrastructure"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPrivacyUserActionsRespectTenantMembership(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tenants, users []string
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, id := range users {
			for _, q := range []string{"DELETE FROM user_tenant_roles WHERE user_id=$1", "DELETE FROM user_tenants WHERE user_id=$1", "DELETE FROM users WHERE id=$1"} {
				if _, err := pool.Exec(cleanup, q, id); err != nil {
					t.Errorf("cleanup user fixture: %v", err)
				}
			}
		}
		for _, id := range tenants {
			if _, err := pool.Exec(cleanup, "DELETE FROM companies WHERE id=$1", id); err != nil {
				t.Errorf("cleanup tenant fixture: %v", err)
			}
		}
	}()
	suffix := time.Now().UnixNano()
	createTenant := func(i int64) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('Privacy CI','Privacy CI',$1) RETURNING id::text", fmt.Sprintf("%014d", (suffix+i)%100000000000000)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, id)
		return id
	}
	createUser := func(kind string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, "INSERT INTO users(email,name,password_hash,active,last_login_at) VALUES ($1,'Privacy fixture','not-a-real-password-hash',true,now()) RETURNING id::text", fmt.Sprintf("privacy-%s-%d@example.invalid", kind, suffix)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users = append(users, id)
		return id
	}
	link := func(user, tenant string, active bool, role string) {
		t.Helper()
		if _, err := pool.Exec(ctx, "INSERT INTO user_tenants(user_id,tenant_id,active) VALUES ($1,$2,$3)", user, tenant, active); err != nil {
			t.Fatal(err)
		}
		tag, err := pool.Exec(ctx, "INSERT INTO user_tenant_roles(user_id,tenant_id,role_id) SELECT $1,$2,id FROM roles WHERE name=$3", user, tenant, role)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("link role: %v", err)
		}
	}
	a, b, c := createTenant(0), createTenant(1), createTenant(2)
	shared, exclusive, admin := createUser("shared"), createUser("exclusive"), createUser("admin")
	link(shared, a, true, "cashier")
	link(shared, b, true, "cashier")
	link(exclusive, a, true, "cashier")
	link(admin, a, true, "admin")
	repo := privacyinfra.NewRepo(pool)
	auth := authinfra.NewUsersRepo(pool, false)
	if err := repo.BlockSubject(ctx, c, "user", shared); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unrelated tenant block: %v", err)
	}
	if err := repo.BlockSubject(ctx, a, "user", shared); err != nil {
		t.Fatal(err)
	}
	for tenant, want := range map[string]bool{a: false, b: true} {
		ok, err := auth.UserHasTenant(ctx, shared, tenant)
		if err != nil || ok != want {
			t.Fatalf("membership active=%v want=%v err=%v", ok, want, err)
		}
		data, err := repo.ExportSubjectData(ctx, tenant, "user", shared)
		if err != nil || data["active"] != want {
			t.Fatalf("scoped export active: %v", err)
		}
		if login, ok := data["last_login_at"].(*string); !ok || login != nil {
			t.Fatal("global last login leaked from a shared identity")
		}
	}
	// Even an inactive second membership blocks global anonymization.
	if err := repo.AnonymizeSubject(ctx, b, "user", shared); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("shared anonymization: %v", err)
	}
	u, err := auth.GetByID(ctx, shared)
	if err != nil || !u.Active || u.Name != "Privacy fixture" {
		t.Fatal("shared identity changed globally")
	}
	if err := repo.BlockSubject(ctx, a, "user", admin); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("admin block: %v", err)
	}
	if err := repo.AnonymizeSubject(ctx, a, "user", admin); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("admin anonymization: %v", err)
	}
	before, err := auth.GetByID(ctx, exclusive)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AnonymizeSubject(ctx, a, "user", exclusive); err != nil {
		t.Fatal(err)
	}
	after, err := auth.GetByID(ctx, exclusive)
	if err != nil || after.Active || after.Name != "Usuario anonimizado" || after.AuthVersion != before.AuthVersion+1 {
		t.Fatal("exclusive identity not safely anonymized")
	}
	ok, err := auth.UserHasTenant(ctx, exclusive, a)
	if err != nil || ok {
		t.Fatal("exclusive membership still active")
	}
}
