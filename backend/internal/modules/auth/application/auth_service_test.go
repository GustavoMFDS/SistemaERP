package application

import (
	"context"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	authdomain "github.com/example/sistemaemgo/internal/modules/auth/domain"
	"github.com/example/sistemaemgo/internal/modules/common"
	"golang.org/x/crypto/bcrypt"
)

func TestRefreshWithSubjectReturnsAuditIdentity(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &fakeUsersRepo{
		user:     authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantID: "tenant-1",
		roles:    []string{"admin"},
	}
	refreshStore := newFakeRefreshStore()
	svc := NewAuthService(testAuthConfig(), users, refreshStore, nil)

	loginResp, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	_, userID, tenantID, err := svc.RefreshWithSubject(context.Background(), loginResp.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshWithSubject returned error: %v", err)
	}
	if userID != "user-1" || tenantID != "tenant-1" {
		t.Fatalf("unexpected audit identity user=%q tenant=%q", userID, tenantID)
	}
}

func TestPermissionCacheIsTenantScoped(t *testing.T) {
	users := &fakeUsersRepo{
		tenantPerms: map[string][]string{
			"user-1:tenant-a": {"audit:read"},
			"user-1:tenant-b": {"product:read"},
		},
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	a, err := svc.GetUserPermissions(context.Background(), "user-1", "tenant-a")
	if err != nil {
		t.Fatalf("tenant-a permissions: %v", err)
	}
	b, err := svc.GetUserPermissions(context.Background(), "user-1", "tenant-b")
	if err != nil {
		t.Fatalf("tenant-b permissions: %v", err)
	}
	if !a["audit:read"] || a["product:read"] {
		t.Fatalf("tenant-a permissions leaked or missing: %#v", a)
	}
	if !b["product:read"] || b["audit:read"] {
		t.Fatalf("tenant-b permissions leaked or missing: %#v", b)
	}
}

func TestPermissionsReflectRoleChangesImmediately(t *testing.T) {
	users := &fakeUsersRepo{
		tenantPerms: map[string][]string{
			"user-1:tenant-a": {"audit:read"},
		},
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	first, err := svc.GetUserPermissions(context.Background(), "user-1", "tenant-a")
	if err != nil {
		t.Fatalf("initial permissions: %v", err)
	}
	if !first["audit:read"] {
		t.Fatalf("expected initial audit permission: %#v", first)
	}

	users.tenantPerms["user-1:tenant-a"] = []string{"product:read"}
	second, err := svc.GetUserPermissions(context.Background(), "user-1", "tenant-a")
	if err != nil {
		t.Fatalf("updated permissions: %v", err)
	}
	if second["audit:read"] || !second["product:read"] {
		t.Fatalf("permission change was not immediate: %#v", second)
	}
}

func TestGetUserInfoUsesAuthenticatedTenant(t *testing.T) {
	allowed := true
	users := &fakeUsersRepo{
		user:          authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", Active: true},
		tenantID:      "tenant-a",
		roles:         []string{"manager"},
		tenantAllowed: &allowed,
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	info, err := svc.GetUserInfo(context.Background(), "user-1", "tenant-b")
	if err != nil {
		t.Fatalf("GetUserInfo returned error: %v", err)
	}
	if info.TenantID != "tenant-b" {
		t.Fatalf("expected authenticated tenant-b, got %q", info.TenantID)
	}
}

func TestLoginFailsWhenTenantMappingMissing(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &fakeUsersRepo{
		user:      authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantErr: common.ErrForbidden,
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	_, _, err = svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != common.ErrForbidden {
		t.Fatalf("expected tenant mapping error, got %v", err)
	}
}

func TestValidateTokenChecksActiveUser(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &fakeUsersRepo{
		user:     authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantID: "tenant-1",
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	loginResp, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	userID, tenantID, err := svc.ValidateToken(context.Background(), loginResp.AccessToken)
	if err != nil {
		t.Fatalf("expected active user token to validate: %v", err)
	}
	if userID != "user-1" || tenantID != "tenant-1" {
		t.Fatalf("unexpected token subject user=%q tenant=%q", userID, tenantID)
	}

	users.user.Active = false
	_, _, err = svc.ValidateToken(context.Background(), loginResp.AccessToken)
	if err != common.ErrInactiveUser {
		t.Fatalf("expected inactive user to be rejected, got %v", err)
	}
}

func TestValidateTokenRejectsRemovedTenantMembership(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	allowed := true
	users := &fakeUsersRepo{
		user:          authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantID:      "tenant-1",
		tenantAllowed: &allowed,
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	loginResp, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	allowed = false
	_, _, err = svc.ValidateToken(context.Background(), loginResp.AccessToken)
	if err != common.ErrForbidden {
		t.Fatalf("expected removed tenant membership to reject access token, got %v", err)
	}
}

func TestRefreshRejectsRemovedTenantMembership(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	allowed := true
	users := &fakeUsersRepo{
		user:          authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantID:      "tenant-1",
		tenantAllowed: &allowed,
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	loginResp, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	allowed = false
	_, _, _, err = svc.RefreshWithSubject(context.Background(), loginResp.RefreshToken)
	if err != common.ErrForbidden {
		t.Fatalf("expected removed tenant membership to reject refresh, got %v", err)
	}
}

func TestRefreshChecksActiveUser(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &fakeUsersRepo{
		user:     authdomain.User{ID: "user-1", Email: "admin@example.com", Name: "Admin", PasswordHash: string(hash), Active: true},
		tenantID: "tenant-1",
	}
	svc := NewAuthService(testAuthConfig(), users, newFakeRefreshStore(), nil)

	loginResp, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	users.user.Active = false

	_, _, _, err = svc.RefreshWithSubject(context.Background(), loginResp.RefreshToken)
	if err != common.ErrInactiveUser {
		t.Fatalf("expected inactive user refresh to be rejected, got %v", err)
	}
}

type fakeUsersRepo struct {
	user          authdomain.User
	tenantID      string
	tenantErr     error
	roles         []string
	tenantPerms   map[string][]string
	tenantAllowed *bool
}

func (f *fakeUsersRepo) GetByEmail(ctx context.Context, email string) (authdomain.User, error) {
	if f.user.Email == email {
		return f.user, nil
	}
	return authdomain.User{}, common.ErrInvalidCredentials
}

func (f *fakeUsersRepo) GetByID(ctx context.Context, id string) (authdomain.User, error) {
	if f.user.ID == id {
		return f.user, nil
	}
	return authdomain.User{}, common.ErrNotFound
}

func (f *fakeUsersRepo) UpdateLastLogin(ctx context.Context, id string) error { return nil }

func (f *fakeUsersRepo) GetDefaultTenantID(ctx context.Context, userID string) (string, error) {
	if f.tenantErr != nil {
		return "", f.tenantErr
	}
	return f.tenantID, nil
}

func (f *fakeUsersRepo) ListUserRoles(ctx context.Context, userID string, tenantID string) ([]string, error) {
	return f.roles, nil
}

func (f *fakeUsersRepo) UserHasTenant(ctx context.Context, userID string, tenantID string) (bool, error) {
	if f.tenantAllowed != nil {
		return *f.tenantAllowed, nil
	}
	return tenantID == f.tenantID, nil
}

func (f *fakeUsersRepo) ListUserPermissions(ctx context.Context, userID string, tenantID string) ([]string, error) {
	return f.tenantPerms[userID+":"+tenantID], nil
}

type fakeRefreshStore struct {
	tokens map[string]string
}

func newFakeRefreshStore() *fakeRefreshStore {
	return &fakeRefreshStore{tokens: map[string]string{}}
}

func (f *fakeRefreshStore) Save(ctx context.Context, tokenID string, userID string, ttlSeconds int64) error {
	f.tokens[tokenID] = userID
	return nil
}

func (f *fakeRefreshStore) Consume(ctx context.Context, tokenID string, userID string) (bool, error) {
	if f.tokens[tokenID] != userID {
		return false, nil
	}
	delete(f.tokens, tokenID)
	return true, nil
}

func (f *fakeRefreshStore) Revoke(ctx context.Context, tokenID string) error {
	delete(f.tokens, tokenID)
	return nil
}

func testAuthConfig() config.Config {
	return config.Config{
		Env:             "test",
		JWTSecret:       "this-is-a-long-test-secret-for-auth-tests",
		JWTIssuer:       "sistemaemgo-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	}
}
