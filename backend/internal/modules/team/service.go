package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrInviteUnavailable = errors.New("invitation expired, used or unavailable")

type Member struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

type Invitation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Service struct {
	db    *pgxpool.Pool
	audit *audit.Service
}

func New(db *pgxpool.Pool, auditService *audit.Service) *Service {
	return &Service{db: db, audit: auditService}
}

func permittedRole(role string) bool {
	return role == "manager" || role == "cashier"
}

func validNewInvite(name, email, role string) bool {
	if !permittedRole(role) || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 ||
		len(email) > 254 || len(email) < 5 || strings.ContainsAny(name, "\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email && !strings.ContainsAny(email, " \t\r\n")
}

func isUniqueError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}


// lockAdminTenant serializes administrative changes and verifies the
// actor's current active admin membership in the same transaction. Route RBAC
// remains mandatory; this second check prevents an internal caller bypassing it.
func lockAdminTenant(ctx context.Context, tx pgx.Tx, tenantID, actorID string) error {
	var companyID string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM companies WHERE id=$1 FOR UPDATE
	`, tenantID).Scan(&companyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return common.ErrForbidden
	}
	if err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_tenant_roles ur
			JOIN roles role ON role.id=ur.role_id AND role.name='admin'
			JOIN user_tenants ut ON ut.user_id=ur.user_id
			  AND ut.tenant_id=ur.tenant_id AND ut.active=true
			JOIN users u ON u.id=ur.user_id AND u.active=true
			WHERE ur.tenant_id=$1 AND ur.user_id=$2
		)
	`, tenantID, actorID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return common.ErrForbidden
	}
	return nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Member, []Invitation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id::text, u.name, u.email::text, ut.active,
			COALESCE((
				SELECT r.name FROM user_tenant_roles tr
				JOIN roles r ON r.id=tr.role_id
				WHERE tr.user_id=u.id AND tr.tenant_id=ut.tenant_id
				ORDER BY CASE r.name WHEN 'admin' THEN 0 WHEN 'manager' THEN 1 ELSE 2 END
				LIMIT 1
			), '')
		FROM user_tenants ut JOIN users u ON u.id=ut.user_id
		WHERE ut.tenant_id=$1
		ORDER BY u.name, u.id
		LIMIT 200
	`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	members := make([]Member, 0)
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.Name, &member.Email, &member.Active, &member.Role); err != nil {
			rows.Close()
			return nil, nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	pending, err := s.db.Query(ctx, `
		SELECT id::text, name, email::text, role_name, expires_at
		FROM staff_invitations
		WHERE tenant_id=$1 AND accepted_at IS NULL AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY created_at DESC, id DESC
		LIMIT 200
	`, tenantID)
	if err != nil {
		return nil, nil, err
	}
	defer pending.Close()
	invitations := make([]Invitation, 0)
	for pending.Next() {
		var item Invitation
		if err := pending.Scan(&item.ID, &item.Name, &item.Email, &item.Role, &item.ExpiresAt); err != nil {
			return nil, nil, err
		}
		invitations = append(invitations, item)
	}
	return members, invitations, pending.Err()
}

// Copy the returned one-time secret over a separate trusted channel. Only
// its SHA-256 digest is persisted, never the raw token or password.
func (s *Service) Invite(ctx context.Context, tenantID, actorID, name, email, role string,
	requestID, ip, userAgent string,
) (Invitation, string, error) {
	name, email = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email))
	if !validNewInvite(name, email, role) {
		return Invitation{}, "", common.ErrValidation
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return Invitation{}, "", err
	}
	digest := sha256.Sum256(token)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Invitation{}, "", err
	}
	defer tx.Rollback(ctx)
	// Serialize per-tenant invitation generation and role modifications.
	if err := lockAdminTenant(ctx, tx, tenantID, actorID); err != nil {
		return Invitation{}, "", err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, email).Scan(&exists); err != nil {
		return Invitation{}, "", err
	}
	if exists {
		// No joining an existing global identity without proof of ownership.
		return Invitation{}, "", common.ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_invitations SET revoked_at=now()
		WHERE tenant_id=$1 AND email=$2 AND accepted_at IS NULL AND revoked_at IS NULL
	`, tenantID, email); err != nil {
		return Invitation{}, "", err
	}
	var item Invitation
	err = tx.QueryRow(ctx, `
		INSERT INTO staff_invitations(tenant_id,email,name,role_name,token_hash,invited_by_user_id,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,now()+interval '48 hours')
		RETURNING id::text,name,email::text,role_name,expires_at
	`, tenantID, email, name, role, digest[:], actorID).
		Scan(&item.ID, &item.Name, &item.Email, &item.Role, &item.ExpiresAt)
	if isUniqueError(err) {
		return Invitation{}, "", common.ErrConflict
	}
	if err != nil {
		return Invitation{}, "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID, Action: "team.invite.create",
		ResourceType: "staff_invitation", ResourceID: item.ID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
		Metadata: map[string]any{"role": role},
	}); err != nil {
		return Invitation{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return Invitation{}, "", err
	}
	return item, hex.EncodeToString(token), nil
}

func (s *Service) Revoke(ctx context.Context, tenantID, actorID, inviteID, requestID, ip, userAgent string) error {
	if _, err := uuid.Parse(inviteID); err != nil {
		return common.ErrValidation
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockAdminTenant(ctx, tx, tenantID, actorID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE staff_invitations SET revoked_at=now()
		WHERE id=$1 AND tenant_id=$2 AND accepted_at IS NULL AND revoked_at IS NULL
	`, inviteID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrNotFound
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID, Action: "team.invite.revoke",
		ResourceType: "staff_invitation", ResourceID: inviteID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Accept is anonymous but requires an unpredictable 256-bit token and a
// strong user-chosen password. Invites cannot link a pre-existing global user
// silently, and the single-use row is locked for concurrent submissions.
func (s *Service) Accept(ctx context.Context, tokenHex, password string, requestID, ip, userAgent string) error {
	if len(tokenHex) != 64 || len(password) < 12 || len(password) > 72 {
		return common.ErrValidation
	}
	raw, err := hex.DecodeString(tokenHex)
	if err != nil || len(raw) != 32 {
		return common.ErrValidation
	}
	digest := sha256.Sum256(raw)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var inviteID, tenantID, email, name, role string
	err = tx.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, email::text, name, role_name
		FROM staff_invitations
		WHERE token_hash=$1 AND accepted_at IS NULL AND revoked_at IS NULL
		  AND expires_at > now()
		FOR UPDATE
	`, digest[:]).Scan(&inviteID, &tenantID, &email, &name, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInviteUnavailable
	}
	if err != nil {
		return err
	}
	if !permittedRole(role) {
		return common.ErrForbidden
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users(email,name,password_hash,active)
		VALUES($1,$2,$3,true) RETURNING id::text
	`, email, name, string(passwordHash)).Scan(&userID)
	if isUniqueError(err) {
		return ErrInviteUnavailable
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id,active) VALUES($1,$2,true)
	`, userID, tenantID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO user_tenant_roles(user_id,tenant_id,role_id)
		SELECT $1,$2,r.id FROM roles r WHERE r.name=$3
	`, userID, tenantID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	if _, err := tx.Exec(ctx, `
		UPDATE staff_invitations SET accepted_at=now() WHERE id=$1
	`, inviteID); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID, Action: "team.invite.accept",
		ResourceType: "user", ResourceID: userID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
		Metadata: map[string]any{"role": role},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) UpdateMember(ctx context.Context, tenantID, actorID, userID, role string,
	active *bool, requestID, ip, userAgent string,
) error {
	if _, err := uuid.Parse(userID); err != nil {
		return common.ErrValidation
	}
	if userID == actorID || (active == nil && !permittedRole(role)) ||
		(active != nil && role != "") {
		return common.ErrValidation
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Lock company to serialize all admin mutations within this tenant.
	if err := lockAdminTenant(ctx, tx, tenantID, actorID); err != nil {
		return err
	}
	var isAdmin bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_tenant_roles ur JOIN roles r ON r.id=ur.role_id
			WHERE ur.tenant_id=$1 AND ur.user_id=$2 AND r.name='admin'
		)
	`, tenantID, userID).Scan(&isAdmin)
	if err != nil {
		return err
	}
	if isAdmin {
		// Owner/admin roles are managed only through controlled provisioning.
		return common.ErrForbidden
	}
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM user_tenants WHERE tenant_id=$1 AND user_id=$2)
	`, tenantID, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return common.ErrNotFound
	}
	action := "team.member.role"
	metadata := map[string]any{"role": role}
	if active != nil {
		action = "team.member.status"
		metadata = map[string]any{"active": *active}
		tag, err := tx.Exec(ctx, `
			UPDATE user_tenants SET active=$3 WHERE tenant_id=$1 AND user_id=$2
		`, tenantID, userID, *active)
		if err != nil || tag.RowsAffected() != 1 {
			if err != nil {
				return err
			}
			return common.ErrNotFound
		}
	} else {
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_tenant_roles WHERE tenant_id=$1 AND user_id=$2
		`, tenantID, userID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO user_tenant_roles(user_id,tenant_id,role_id)
			SELECT $1,$2,r.id FROM roles r WHERE r.name=$3
		`, userID, tenantID, role)
		if err != nil || tag.RowsAffected() != 1 {
			if err != nil {
				return err
			}
			return common.ErrConflict
		}
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID, Action: action,
		ResourceType: "user", ResourceID: userID, RequestID: requestID,
		IP: ip, UserAgent: userAgent, Metadata: metadata,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
