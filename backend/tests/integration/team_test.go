//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/team"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStaffInviteActivationRBACAndTenantRevocation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var ownerID string
	if err := db.QueryRow(ctx, `SELECT id::text FROM users WHERE email='admin@sistema.local'`).Scan(&ownerID); err != nil {
		t.Fatalf("admin seed required: %v", err)
	}
	var tenants []string
	for i := 0; i < 2; i++ {
		var id string
		cnpj := fmt.Sprintf("%014d", (time.Now().UnixNano()+int64(i))%100000000000000)
		if err := db.QueryRow(ctx, `
			INSERT INTO companies(legal_name,cnpj)
			VALUES('Team Test', $1) RETURNING id::text
		`, cnpj).Scan(&id); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, id)
		if _, err := db.Exec(ctx, `
			INSERT INTO user_tenants(user_id,tenant_id,active)
			VALUES($1,$2,true)
		`, ownerID, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `
			INSERT INTO user_tenant_roles(user_id,tenant_id,role_id)
			SELECT $1,$2,r.id FROM roles r WHERE r.name='admin'
		`, ownerID, id); err != nil {
			t.Fatal(err)
		}
	}
	tenantA, tenantB := tenants[0], tenants[1]
	var staffID string
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for _, tenant := range tenants {
			_, _ = db.Exec(cleanup, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenant)
			_, _ = db.Exec(cleanup, `DELETE FROM staff_invitations WHERE tenant_id=$1`, tenant)
			_, _ = db.Exec(cleanup, `DELETE FROM user_tenant_roles WHERE tenant_id=$1`, tenant)
			_, _ = db.Exec(cleanup, `DELETE FROM user_tenants WHERE tenant_id=$1`, tenant)
			_, _ = db.Exec(cleanup, `DELETE FROM companies WHERE id=$1`, tenant)
		}
		if staffID != "" {
			_, _ = db.Exec(cleanup, `DELETE FROM users WHERE id=$1`, staffID)
		}
	}()
	svc := team.New(db, audit.New(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	repo := authinfra.NewUsersRepo(db, false)
	email := fmt.Sprintf("staff-%d@example.test", time.Now().UnixNano())
	invitation, token, err := svc.Invite(ctx, tenantA, ownerID, "Funcionária Teste", email,
		"cashier", "", "", "")
	if err != nil || len(token) != 64 || invitation.Email != email {
		t.Fatalf("creating private invite failed: %+v tokenLength=%d err=%v", invitation, len(token), err)
	}
	otherMembers, otherInvites, err := svc.List(ctx, tenantB)
	if err != nil || len(otherInvites) != 0 || len(otherMembers) != 1 || otherMembers[0].ID != ownerID {
		t.Fatalf("tenant B saw tenant A invitation: %v %+v %+v", err, otherMembers, otherInvites)
	}
	if err := svc.Revoke(ctx, tenantB, ownerID, invitation.ID, "", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("tenant B revoked a different company's invitation: %v", err)
	}
	password := "UnicaSenhaForte2026!?"
	if err := svc.Accept(ctx, token, password, "", "", ""); err != nil {
		t.Fatalf("acceptance failed: %v", err)
	}
	if _, _, err := svc.Invite(ctx, tenantA, ownerID, "Outra Conta", email,
		"cashier", "", "", ""); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("existing identity cannot be linked to another account implicitly: %v", err)
	}
	revoked, revokedToken, err := svc.Invite(ctx, tenantA, ownerID, "Outra Pessoa",
		fmt.Sprintf("revoked-%d@example.test", time.Now().UnixNano()), "manager", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke(ctx, tenantA, ownerID, revoked.ID, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(ctx, revokedToken, password, "", "", ""); !errors.Is(err, team.ErrInviteUnavailable) {
		t.Fatalf("revoked invitation must not be accepted: %v", err)
	}
	expired, expiredToken, err := svc.Invite(ctx, tenantA, ownerID, "Convite Expirado",
		fmt.Sprintf("expired-%d@example.test", time.Now().UnixNano()), "cashier", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE staff_invitations SET expires_at=now()-interval '1 minute'
		WHERE id=$1 AND tenant_id=$2
	`, expired.ID, tenantA); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(ctx, expiredToken, password, "", "", ""); !errors.Is(err, team.ErrInviteUnavailable) {
		t.Fatalf("expired invitation must not be accepted: %v", err)
	}
	if err := svc.Accept(ctx, token, password, "", "", ""); !errors.Is(err, team.ErrInviteUnavailable) {
		t.Fatalf("same token must never activate twice: %v", err)
	}
	members, invites, err := svc.List(ctx, tenantA)
	if err != nil || len(invites) != 0 || len(members) != 2 {
		t.Fatalf("activated membership/consumed invite invalid: %+v %+v err=%v", members, invites, err)
	}
	for _, member := range members {
		if member.Email == email {
			staffID = member.ID
			if !member.Active || member.Role != "cashier" {
				t.Fatalf("invited staff role is wrong: %+v", member)
			}
		}
	}
	if staffID == "" {
		t.Fatalf("new employee missing after acceptance: %+v", members)
	}
	has, err := repo.UserHasTenant(ctx, staffID, tenantA)
	if err != nil || !has {
		t.Fatalf("new employee lacks access: %v %v", has, err)
	}
	has, err = repo.UserHasTenant(ctx, staffID, tenantB)
	if err != nil || has {
		t.Fatalf("employee leaked to tenant B: %v %v", has, err)
	}
	if err := svc.UpdateMember(ctx, tenantB, ownerID, staffID, "manager", nil, "", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("cross-company role change was allowed: %v", err)
	}
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "admin", nil, "", "", ""); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("privilege escalation to admin was allowed: %v", err)
	}
	if err := svc.UpdateMember(ctx, tenantA, ownerID, ownerID, "cashier", nil, "", "", ""); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("administrator must not change own role: %v", err)
	}
	if err := svc.UpdateMember(ctx, tenantA, staffID, staffID, "manager", nil, "", "", ""); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("self-promotion was allowed: %v", err)
	}
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "manager", nil, "", "", ""); err != nil {
		t.Fatalf("role change: %v", err)
	}
	// A manager has operational permissions but must not be able to mutate
	// staff through direct service callers, even if a route guard regresses.
	if _, _, err := svc.Invite(ctx, tenantA, staffID, "Privilegio Indevido",
		fmt.Sprintf("escalation-%d@example.test", time.Now().UnixNano()),
		"cashier", "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("manager created invitation without admin rights: %v", err)
	}
	if err := svc.Revoke(ctx, tenantA, staffID, expired.ID, "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("manager revoked invitation without admin rights: %v", err)
	}
	if err := svc.UpdateMember(ctx, tenantA, staffID, ownerID, "cashier", nil, "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("manager changed admin role without privileges: %v", err)
	}

	perms, err := repo.ListUserPermissions(ctx, staffID, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	var finance bool
	var manageTeam bool
	for _, permission := range perms {
		if permission == "finance:read" {
			finance = true
		}
		if permission == "team:manage" {
			manageTeam = true
		}
	}
	if !finance || manageTeam {
		t.Fatalf("manager should have financial access but not team:manage: %+v", perms)
	}
	inactive := false
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "", &inactive, "", "", ""); err != nil {
		t.Fatalf("disable tenant membership: %v", err)
	}
	has, err = repo.UserHasTenant(ctx, staffID, tenantA)
	if err != nil || has {
		t.Fatalf("disabled membership still authorizes existing token: %v %v", has, err)
	}
	perms, err = repo.ListUserPermissions(ctx, staffID, tenantA)
	if err != nil || len(perms) != 0 {
		t.Fatalf("disabled member still has effective RBAC: %+v err=%v", perms, err)
	}
	active := true
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "", &active, "", "", ""); err != nil {
		t.Fatalf("reenable tenant membership: %v", err)
	}
	has, err = repo.UserHasTenant(ctx, staffID, tenantA)
	if err != nil || !has {
		t.Fatalf("reactivated membership not authorized: %v %v", has, err)
	}
	// Stored invite token is a hash, not the recoverable token itself.
	raw, _ := hex.DecodeString(token)
	digest := sha256.Sum256(raw)
	var stored []byte
	if err := db.QueryRow(ctx, `
		SELECT token_hash FROM staff_invitations WHERE id=$1
	`, invitation.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(stored) != hex.EncodeToString(digest[:]) {
		t.Fatal("invitation does not store the hash of token")
	}
}
