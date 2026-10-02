package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	authdomain "github.com/example/sistemaemgo/internal/modules/auth/domain"
	"golang.org/x/crypto/bcrypt"
)

func TestPublicTokenResponseDoesNotExposeRefreshToken(t *testing.T) {
	body := publicTokenResponse(authapp.TokenResponse{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		TokenType:        "Bearer",
		ExpiresIn:        900,
		RefreshExpiresIn: 3600,
	})

	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "refresh-token") || strings.Contains(got, "refresh_token") {
		t.Fatalf("public auth response leaked refresh token: %s", got)
	}
	if !strings.Contains(got, "access-token") {
		t.Fatalf("public auth response omitted access token: %s", got)
	}
}

func TestRefreshCookieUsesHostPrefixOnlyForProdLikeHTTPS(t *testing.T) {
	localCfg := config.Config{Env: "test"}
	localRec := httptest.NewRecorder()
	setRefreshCookie(localRec, localCfg, "local-token", time.Hour)
	localHeader := localRec.Header().Get("Set-Cookie")
	if !strings.Contains(localHeader, localRefreshCookieName+"=") {
		t.Fatalf("local refresh cookie name=%q, want %q", localHeader, localRefreshCookieName)
	}
	if strings.Contains(localHeader, "__Host-") || strings.Contains(localHeader, "Secure") {
		t.Fatalf("local HTTP cookie must not use reserved __Host- prefix or Secure: %q", localHeader)
	}
	if !strings.Contains(localHeader, "HttpOnly") || !strings.Contains(localHeader, "SameSite=Strict") {
		t.Fatalf("local refresh cookie lost security attributes: %q", localHeader)
	}

	prodCfg := config.Config{Env: "staging"}
	prodRec := httptest.NewRecorder()
	setRefreshCookie(prodRec, prodCfg, "prod-token", time.Hour)
	prodHeader := prodRec.Header().Get("Set-Cookie")
	if !strings.Contains(prodHeader, "__Host-refresh_token=") ||
		!strings.Contains(prodHeader, "Secure") ||
		!strings.Contains(prodHeader, "HttpOnly") ||
		!strings.Contains(prodHeader, "SameSite=Strict") {
		t.Fatalf("prod-like refresh cookie must keep __Host- security contract: %q", prodHeader)
	}
}

func TestRefreshFailureExpiresInvalidCookie(t *testing.T) {
	cfg := config.Config{
		Env:             "test",
		JWTSecret:       "this-is-a-long-test-secret-for-handler-tests",
		JWTIssuer:       "sistemaemgo-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	}
	users := &logoutUsersRepo{}
	svc := authapp.NewAuthService(cfg, users, &logoutRefreshStore{tokens: map[string]string{}}, slog.Default())
	h := NewAuthHandler(cfg, svc, nil, nil, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName(cfg), Value: "invalid-token", Path: "/"})
	rec := httptest.NewRecorder()
	h.Refresh(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh status=%d, want 401", rec.Code)
	}
	cleared := false
	for _, header := range rec.Header().Values("Set-Cookie") {
		if strings.Contains(header, refreshCookieName(cfg)) && strings.Contains(header, "Max-Age=0") {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("invalid refresh must expire the browser cookie")
	}
}

func TestLogoutRequiresSuccessfulRefreshRevocation(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("strong-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	users := &logoutUsersRepo{
		user: authdomain.User{
			ID:           "11111111-1111-1111-1111-111111111111",
			Email:        "admin@example.com",
			Name:         "Admin",
			PasswordHash: string(hash),
			Active:       true,
		},
	}
	store := &logoutRefreshStore{tokens: map[string]string{}}
	cfg := config.Config{
		Env:             "test",
		JWTSecret:       "this-is-a-long-test-secret-for-handler-tests",
		JWTIssuer:       "sistemaemgo-test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	}
	svc := authapp.NewAuthService(cfg, users, store, slog.Default())
	login, _, err := svc.Login(context.Background(), "admin@example.com", "strong-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(store.tokens) != 1 {
		t.Fatalf("expected one stored refresh token, got %d", len(store.tokens))
	}

	h := NewAuthHandler(cfg, svc, nil, nil, slog.Default())
	store.revokeErr = errors.New("redis unavailable")

	failedReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	failedReq.AddCookie(&http.Cookie{Name: refreshCookieName(cfg), Value: login.RefreshToken, Path: "/"})
	failedRec := httptest.NewRecorder()
	h.Logout(failedRec, failedReq)

	if failedRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed logout status=%d, want 503", failedRec.Code)
	}
	for _, header := range failedRec.Header().Values("Set-Cookie") {
		if strings.Contains(header, refreshCookieName(cfg)) && strings.Contains(header, "Max-Age=0") {
			t.Fatalf("failed logout must not expire refresh cookie: %q", header)
		}
	}
	if len(store.tokens) != 1 {
		t.Fatalf("failed revocation must preserve server token, got %d", len(store.tokens))
	}

	store.revokeErr = nil
	okReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	okReq.AddCookie(&http.Cookie{Name: refreshCookieName(cfg), Value: login.RefreshToken, Path: "/"})
	okRec := httptest.NewRecorder()
	h.Logout(okRec, okReq)

	if okRec.Code != http.StatusOK {
		t.Fatalf("successful logout status=%d, want 200", okRec.Code)
	}
	cleared := false
	for _, header := range okRec.Header().Values("Set-Cookie") {
		if strings.Contains(header, refreshCookieName(cfg)) && strings.Contains(header, "Max-Age=0") {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("successful logout must expire refresh cookie")
	}
	if len(store.tokens) != 0 {
		t.Fatalf("successful revocation must remove server token, got %d", len(store.tokens))
	}
}

type logoutUsersRepo struct {
	user authdomain.User
}

func (f *logoutUsersRepo) GetByEmail(context.Context, string) (authdomain.User, error) {
	return f.user, nil
}
func (f *logoutUsersRepo) GetByID(context.Context, string) (authdomain.User, error) {
	return f.user, nil
}
func (f *logoutUsersRepo) UpdateLastLogin(context.Context, string) error { return nil }
func (f *logoutUsersRepo) GetDefaultTenantID(context.Context, string) (string, error) {
	return "22222222-2222-2222-2222-222222222222", nil
}
func (f *logoutUsersRepo) UserHasTenant(context.Context, string, string) (bool, error) {
	return true, nil
}
func (f *logoutUsersRepo) ListUserRoles(context.Context, string, string) ([]string, error) {
	return []string{"admin"}, nil
}
func (f *logoutUsersRepo) ListUserPermissions(context.Context, string, string) ([]string, error) {
	return []string{"product:read"}, nil
}

type logoutRefreshStore struct {
	tokens    map[string]string
	revokeErr error
}

func (f *logoutRefreshStore) Save(_ context.Context, tokenID, userID string, _ int64) error {
	f.tokens[tokenID] = userID
	return nil
}
func (f *logoutRefreshStore) Consume(_ context.Context, tokenID, userID string) (bool, error) {
	if f.tokens[tokenID] != userID {
		return false, nil
	}
	delete(f.tokens, tokenID)
	return true, nil
}
func (f *logoutRefreshStore) Revoke(_ context.Context, tokenID string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	delete(f.tokens, tokenID)
	return nil
}
