//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/team"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
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
	if err != nil || len(invites) != 1 || invites[0].ID != expired.ID || len(members) != 2 {
		t.Fatalf("activated membership / expired invite visibility invalid: %+v %+v err=%v", members, invites, err)
	}
	// An expired invitation stays visible so the original store admin can
	// explicitly revoke and release its globally reserved username.
	if err := svc.Revoke(ctx, tenantA, ownerID, expired.ID, "", "", ""); err != nil {
		t.Fatalf("expired invite must be revocable: %v", err)
	}
	_, invites, err = svc.List(ctx, tenantA)
	if err != nil || len(invites) != 0 {
		t.Fatalf("revoked expired invite still visible: %+v err=%v", invites, err)
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
	// Development's legacy tenant fallback must not resurrect an employee
	// suspended from the only company to which the identity is linked.
	devRepo := authinfra.NewUsersRepo(db, true)
	if _, err := devRepo.GetDefaultTenantID(ctx, staffID); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("suspended employee gained access through legacy tenant fallback: %v", err)
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
	// The password-recovery flow must be verified against real PostgreSQL
	// constraints and the employee's scoped membership, not a mock.
	testStaffPasswordRecovery(t, ctx, db, svc, ownerID, staffID, tenantA, tenantB)

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

func testStaffPasswordRecovery(
	t *testing.T, ctx context.Context, db *pgxpool.Pool, svc *team.Service,
	ownerID, staffID, tenantA, tenantB string,
) {
	t.Helper()
	strongPassword := "NovaSenhaFuncionario2026!"
	invalidPassword := "curta123"
	// Invalid token input must look exactly like an expired or used link,
	// not expose a different error path.
	for _, invalidToken := range []string{"", "not-hex", strings.Repeat("z", 64)} {
		if err := svc.CompletePasswordReset(ctx, invalidToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
			t.Fatalf("invalid token produced distinguishable response: %v", err)
		}
	}
	if _, _, err := svc.IssuePasswordReset(ctx, tenantB, ownerID, staffID, "", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("other company issued password reset: %v", err)
	}
	if _, _, err := svc.IssuePasswordReset(ctx, tenantA, staffID, staffID, "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("employee attempted self-service recovery: %v", err)
	}
	if _, _, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, ownerID, "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("admin must not reset own password using team flow: %v", err)
	}
	first, firstToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil || len(firstToken) != 64 {
		t.Fatalf("issue recovery link failed: %+v err=%v", first, err)
	}
	pendingA, err := svc.ListPasswordResets(ctx, tenantA, ownerID)
	if err != nil || len(pendingA) != 1 || pendingA[0].ID != first.ID {
		t.Fatalf("shop admin cannot see own pending link metadata: %+v err=%v", pendingA, err)
	}
	pendingB, err := svc.ListPasswordResets(ctx, tenantB, ownerID)
	if err != nil || len(pendingB) != 0 {
		t.Fatalf("another shop saw pending reset metadata: %+v err=%v", pendingB, err)
	}
	if _, err := svc.ListPasswordResets(ctx, tenantA, staffID); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("non-admin saw password reset metadata: %v", err)
	}
	var stored []byte
	if err := db.QueryRow(ctx, `SELECT token_hash FROM staff_password_resets WHERE id=$1`, first.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(firstToken)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if !bytes.Equal(stored, digest[:]) {
		t.Fatal("password reset persisted a token other than its SHA-256 digest")
	}
	second, secondToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatalf("reissue password reset failed: %v", err)
	}
	if err := svc.CompletePasswordReset(ctx, firstToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("superseded token accepted: %v", err)
	}
	if err := svc.RevokePasswordReset(ctx, tenantB, ownerID, second.ID, "", "", ""); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("cross-tenant reset revocation succeeded: %v", err)
	}
	if err := svc.RevokePasswordReset(ctx, tenantA, staffID, second.ID, "", "", ""); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("non-admin revoked reset: %v", err)
	}
	if err := svc.RevokePasswordReset(ctx, tenantA, ownerID, second.ID, "", "", ""); err != nil {
		t.Fatalf("admin could not revoke reset: %v", err)
	}
	if err := svc.CompletePasswordReset(ctx, secondToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("revoked token accepted: %v", err)
	}
	expired, expiredToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE staff_password_resets
		SET created_at=now()-interval '3 hours', expires_at=now()-interval '2 hours'
		WHERE id=$1
	`, expired.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, expiredToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("expired password reset accepted: %v", err)
	}
	link, token, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, token, invalidPassword, "", "", ""); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("short password accepted: %v", err)
	}
	var beforeVersion int64
	if err := db.QueryRow(ctx, `SELECT auth_version FROM users WHERE id=$1`, staffID).Scan(&beforeVersion); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, token, strongPassword, "", "", ""); err != nil {
		t.Fatalf("valid password reset failed: %v", err)
	}
	if err := svc.CompletePasswordReset(ctx, token, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("reused token accepted: %v", err)
	}
	var actualVersion int64
	var passwordHash string
	if err := db.QueryRow(ctx,
		`SELECT auth_version,password_hash FROM users WHERE id=$1`, staffID).
		Scan(&actualVersion, &passwordHash); err != nil {
		t.Fatal(err)
	}
	if actualVersion != beforeVersion+1 || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(strongPassword)) != nil {
		t.Fatalf("password or auth_version not updated atomically: %d => %d", beforeVersion, actualVersion)
	}
	if err := svc.RevokePasswordReset(ctx, tenantA, ownerID, link.ID, "", "", ""); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("consumed link was revoked as if pending: %v", err)
	}
	// A suspended membership remains suspended after password recovery.
	inactive := false
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "", &inactive, "", "", ""); err != nil {
		t.Fatal(err)
	}
	_, suspendedToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, suspendedToken, strongPassword, "", "", ""); err != nil {
		t.Fatalf("suspended employee should be able to set password with admin approval: %v", err)
	}
	var stillSuspended bool
	if err := db.QueryRow(ctx,
		`SELECT active FROM user_tenants WHERE user_id=$1 AND tenant_id=$2`, staffID, tenantA).Scan(&stillSuspended); err != nil {
		t.Fatal(err)
	}
	if stillSuspended {
		t.Fatal("password recovery reactivated a suspended employee")
	}
	active := true
	if err := svc.UpdateMember(ctx, tenantA, ownerID, staffID, "", &active, "", "", ""); err != nil {
		t.Fatal(err)
	}
	// A link issued before an administrative privilege upgrade cannot reset
	// the upgraded account.
	_, toAdminToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_tenant_roles(user_id,tenant_id,role_id)
		SELECT $1,$2,id FROM roles WHERE name='admin'
	`, staffID, tenantA); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, toAdminToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("link allowed password reset after admin promotion: %v", err)
	}
	if _, err := db.Exec(ctx, `
		DELETE FROM user_tenant_roles WHERE user_id=$1 AND tenant_id=$2
		  AND role_id IN (SELECT id FROM roles WHERE name='admin')
	`, staffID, tenantA); err != nil {
		t.Fatal(err)
	}
	// A once-valid reset link must not survive a subsequent association
	// with another company, because the password belongs to the user.
	_, transferToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id,active) VALUES($1,$2,false)
	`, staffID, tenantB); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompletePasswordReset(ctx, transferToken, strongPassword, "", "", ""); !errors.Is(err, team.ErrPasswordResetUnavailable) {
		t.Fatalf("reset link remained valid after second company association: %v", err)
	}
	if _, _, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", ""); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("shared identity accepted a tenant-local password reset: %v", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM user_tenants WHERE user_id=$1 AND tenant_id=$2`, staffID, tenantB); err != nil {
		t.Fatal(err)
	}
	// Two simultaneous submissions of a single-use link must not write
	// the password twice or advance auth_version more than once.
	_, parallelToken, err := svc.IssuePasswordReset(ctx, tenantA, ownerID, staffID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var versionBeforeParallel int64
	if err := db.QueryRow(ctx, `SELECT auth_version FROM users WHERE id=$1`, staffID).Scan(&versionBeforeParallel); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			results <- svc.CompletePasswordReset(ctx, parallelToken, strongPassword, "", "", "")
		}()
	}
	close(start)
	succeeded, unavailable := 0, 0
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, team.ErrPasswordResetUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent reset error: %v", err)
		}
	}
	if succeeded != 1 || unavailable != 1 {
		t.Fatalf("same link accepted in concurrent requests: %d success / %d unavailable", succeeded, unavailable)
	}
	var versionAfterParallel int64
	if err := db.QueryRow(ctx, `SELECT auth_version FROM users WHERE id=$1`, staffID).Scan(&versionAfterParallel); err != nil {
		t.Fatal(err)
	}
	if versionAfterParallel != versionBeforeParallel+1 {
		t.Fatalf("concurrent reset changed credential version more than once: %d => %d",
			versionBeforeParallel, versionAfterParallel)
	}
}
