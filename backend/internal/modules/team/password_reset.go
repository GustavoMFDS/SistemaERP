package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordResetUnavailable deliberately does not distinguish a bad,
// expired, cancelled or already-consumed link.
var ErrPasswordResetUnavailable = errors.New("password recovery link unavailable")

type PasswordResetLink struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IssuePasswordReset is only for a verified admin of the employee's shop.
// Never let one tenant reset a user also belonging to another company;
// password and JWT version are global to the user identity.
func (s *Service) IssuePasswordReset(
	ctx context.Context, tenantID, actorID, userID, requestID, ip, userAgent string,
) (PasswordResetLink, string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return PasswordResetLink{}, "", common.ErrValidation
	}
	if userID == actorID {
		return PasswordResetLink{}, "", common.ErrForbidden
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return PasswordResetLink{}, "", err
	}
	hash := sha256.Sum256(token)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PasswordResetLink{}, "", err
	}
	defer tx.Rollback(ctx)
	if err := lockAdminTenant(ctx, tx, tenantID, actorID); err != nil {
		return PasswordResetLink{}, "", err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE id=$1 FOR UPDATE`, userID).
		Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return PasswordResetLink{}, "", common.ErrNotFound
	} else if err != nil {
		return PasswordResetLink{}, "", err
	}
	var inShop, shared bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM user_tenants ut
		  JOIN user_tenant_roles utr ON utr.user_id=ut.user_id AND utr.tenant_id=ut.tenant_id
		  JOIN roles r ON r.id=utr.role_id AND r.name IN ('cashier','manager')
		  WHERE ut.user_id=$1 AND ut.tenant_id=$2
		), (SELECT count(*)>1 FROM user_tenants WHERE user_id=$1)
	`, userID, tenantID).Scan(&inShop, &shared)
	if err != nil {
		return PasswordResetLink{}, "", err
	}
	if !inShop {
		return PasswordResetLink{}, "", common.ErrNotFound
	}
	if shared {
		return PasswordResetLink{}, "", common.ErrConflict
	}
	// Explicitly protect admin even when an account has multiple roles.
	var isAdmin bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_tenant_roles ur
			JOIN roles r ON r.id=ur.role_id AND r.name='admin'
			WHERE ur.user_id=$1 AND ur.tenant_id=$2
		)
	`, userID, tenantID).Scan(&isAdmin); err != nil {
		return PasswordResetLink{}, "", err
	}
	if isAdmin {
		return PasswordResetLink{}, "", common.ErrForbidden
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_password_resets SET revoked_at=now()
		WHERE user_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL
	`, userID); err != nil {
		return PasswordResetLink{}, "", err
	}
	var link PasswordResetLink
	err = tx.QueryRow(ctx, `
		INSERT INTO staff_password_resets(tenant_id,user_id,token_hash,requested_by_user_id,expires_at)
		VALUES ($1,$2,$3,$4,now()+interval '30 minutes')
		RETURNING id::text,expires_at
	`, tenantID, userID, hash[:], actorID).Scan(&link.ID, &link.ExpiresAt)
	if err != nil {
		return PasswordResetLink{}, "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID,
		Action: "team.password_reset.issue",
		ResourceType: "staff_password_reset", ResourceID: link.ID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
		Metadata: map[string]any{"employee_id": userID},
	}); err != nil {
		return PasswordResetLink{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return PasswordResetLink{}, "", err
	}
	// Never persist, log or resend the raw token.
	return link, hex.EncodeToString(token), nil
}

func (s *Service) CompletePasswordReset(
	ctx context.Context, tokenHex, password, requestID, ip, userAgent string,
) error {
	if len(tokenHex) != 64 || len(password) < 12 || len(password) > 72 {
		return common.ErrValidation
	}
	raw, err := hex.DecodeString(tokenHex)
	if err != nil || len(raw) != 32 {
		return common.ErrValidation
	}
	hash := sha256.Sum256(raw)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id, tenantID, userID string
	err = tx.QueryRow(ctx, `
		SELECT id::text,tenant_id::text,user_id::text FROM staff_password_resets
		WHERE token_hash=$1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>now()
		FOR UPDATE
	`, hash[:]).Scan(&id, &tenantID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPasswordResetUnavailable
	}
	if err != nil {
		return err
	}
	var membershipCount, originalTenantCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE tenant_id=$2)
		FROM user_tenants WHERE user_id=$1
	`, userID, tenantID).Scan(&membershipCount, &originalTenantCount)
	if err != nil {
		return err
	}
	// Block both new multi-tenant links and transfers after issuance. A reset
	// link cannot modify the password of an identity in another company.
	if membershipCount != 1 || originalTenantCount != 1 {
		return ErrPasswordResetUnavailable
	}
	tag, err := tx.Exec(ctx, `
		UPDATE users SET password_hash=$1,auth_version=auth_version+1,updated_at=now()
		WHERE id=$2
	`, string(passwordHash), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrPasswordResetUnavailable
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_password_resets SET consumed_at=now() WHERE id=$1
	`, id); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID,
		Action: "team.password_reset.complete",
		ResourceType: "staff_password_reset", ResourceID: id,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
