package application

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	cfg      config.Config
	users    UsersRepository
	refresh  RefreshTokenStore
	logger   *slog.Logger
	permsMu  sync.RWMutex
	perms    map[string]permissionCacheEntry
	permsTTL time.Duration
}

type permissionCacheEntry struct {
	perms     map[string]bool
	expiresAt time.Time
}

func NewAuthService(cfg config.Config, users UsersRepository, refresh RefreshTokenStore, logger *slog.Logger) *AuthService {
	return &AuthService{cfg: cfg, users: users, refresh: refresh, logger: logger, perms: map[string]permissionCacheEntry{}, permsTTL: 30 * time.Second}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (TokenResponse, AuthUserInfo, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// mitigate timing
		slowEqual(password, "")
		return TokenResponse{}, AuthUserInfo{}, common.ErrInvalidCredentials
	}
	if !u.Active {
		return TokenResponse{}, AuthUserInfo{}, common.ErrInactiveUser
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return TokenResponse{}, AuthUserInfo{}, common.ErrInvalidCredentials
	}
	_ = s.users.UpdateLastLogin(ctx, u.ID)

	tenantID, err := s.users.GetDefaultTenantID(ctx, u.ID)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	if tenantID == "" {
		return TokenResponse{}, AuthUserInfo{}, common.ErrInvalidCredentials
	}

	roles, _ := s.users.ListUserRoles(ctx, u.ID, tenantID)
	accessTok, accessExp, err := s.issueAccessToken(u.ID, tenantID)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, u.ID, tenantID)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	return TokenResponse{
			AccessToken:      accessTok,
			RefreshToken:     refreshTok,
			TokenType:        "Bearer",
			ExpiresIn:        int64(time.Until(accessExp).Seconds()),
			RefreshExpiresIn: int64(time.Until(refreshExp).Seconds()),
		}, AuthUserInfo{
			ID: u.ID, Email: u.Email, Name: u.Name, TenantID: tenantID, Roles: roles,
		}, nil
}

func (s *AuthService) issueAccessToken(userID, tenantID string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.cfg.AccessTokenTTL)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    s.cfg.JWTIssuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}, Type: "access", TenantID: tenantID}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.JWTSecret))
	return signed, exp, err
}

func (s *AuthService) issueRefreshToken(ctx context.Context, userID, tenantID string) (string, time.Time, error) {
	if s.refresh == nil {
		return "", time.Time{}, common.ErrInvalidCredentials
	}
	now := time.Now()
	exp := now.Add(s.cfg.RefreshTokenTTL)
	// jti: 32 random bytes hex => 64 chars
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	jti := hex.EncodeToString(b)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    s.cfg.JWTIssuer,
		Subject:   userID,
		ID:        jti,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}, Type: "refresh", TenantID: tenantID}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.refresh.Save(ctx, jti, userID, int64(time.Until(exp).Seconds())); err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (userID string, tenantID string, err error) {
	parsed, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return "", "", err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return "", "", common.ErrInvalidCredentials
	}
	if claims.Issuer != s.cfg.JWTIssuer {
		return "", "", common.ErrInvalidCredentials
	}
	if claims.Type != "access" {
		return "", "", common.ErrInvalidCredentials
	}
	if claims.Subject == "" || claims.TenantID == "" {
		return "", "", common.ErrInvalidCredentials
	}
	if err := s.ensureUserActive(ctx, claims.Subject); err != nil {
		return "", "", err
	}
	return claims.Subject, claims.TenantID, nil
}

func (s *AuthService) Refresh(ctx context.Context, tokenStr string) (TokenResponse, error) {
	resp, _, _, err := s.RefreshWithSubject(ctx, tokenStr)
	return resp, err
}

func (s *AuthService) RefreshWithSubject(ctx context.Context, tokenStr string) (TokenResponse, string, string, error) {
	claims, err := s.validateRefreshToken(tokenStr)
	if err != nil {
		return TokenResponse{}, "", "", err
	}
	if s.refresh == nil {
		return TokenResponse{}, "", "", common.ErrInvalidCredentials
	}
	ok, err := s.refresh.Consume(ctx, claims.ID, claims.Subject)
	if err != nil {
		return TokenResponse{}, "", "", err
	}
	if !ok {
		return TokenResponse{}, "", "", common.ErrInvalidCredentials
	}
	if err := s.ensureUserActive(ctx, claims.Subject); err != nil {
		return TokenResponse{}, "", "", err
	}
	accessTok, accessExp, err := s.issueAccessToken(claims.Subject, claims.TenantID)
	if err != nil {
		return TokenResponse{}, "", "", err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, claims.Subject, claims.TenantID)
	if err != nil {
		return TokenResponse{}, "", "", err
	}
	return TokenResponse{
		AccessToken:      accessTok,
		RefreshToken:     refreshTok,
		TokenType:        "Bearer",
		ExpiresIn:        int64(time.Until(accessExp).Seconds()),
		RefreshExpiresIn: int64(time.Until(refreshExp).Seconds()),
	}, claims.Subject, claims.TenantID, nil
}

func (s *AuthService) IdentifyRefreshToken(tokenStr string) (userID string, tenantID string, err error) {
	claims, err := s.validateRefreshToken(tokenStr)
	if err != nil {
		return "", "", err
	}
	return claims.Subject, claims.TenantID, nil
}

func (s *AuthService) Logout(ctx context.Context, tokenStr string) error {
	claims, err := s.validateRefreshToken(tokenStr)
	if err != nil {
		return nil
	}
	if s.refresh == nil {
		return nil
	}
	return s.refresh.Revoke(ctx, claims.ID)
}

func (s *AuthService) validateRefreshToken(tokenStr string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, common.ErrInvalidCredentials
	}
	if claims.Issuer != s.cfg.JWTIssuer {
		return nil, common.ErrInvalidCredentials
	}
	if claims.Type != "refresh" {
		return nil, common.ErrInvalidCredentials
	}
	if claims.ID == "" || claims.Subject == "" || claims.TenantID == "" {
		return nil, common.ErrInvalidCredentials
	}
	return claims, nil
}

func (s *AuthService) GetUserPermissions(ctx context.Context, userID string, tenantID string) (map[string]bool, error) {
	key := userID + ":" + tenantID
	now := time.Now()
	s.permsMu.RLock()
	if cached, ok := s.perms[key]; ok && now.Before(cached.expiresAt) {
		out := clonePerms(cached.perms)
		s.permsMu.RUnlock()
		return out, nil
	}
	s.permsMu.RUnlock()

	perms, err := s.users.ListUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(perms))
	for _, p := range perms {
		m[p] = true
	}
	s.permsMu.Lock()
	s.perms[key] = permissionCacheEntry{perms: clonePerms(m), expiresAt: now.Add(s.permsTTL)}
	s.permsMu.Unlock()
	return m, nil
}

func (s *AuthService) InvalidateUserPermissions(userID string) {
	s.permsMu.Lock()
	defer s.permsMu.Unlock()
	prefix := userID + ":"
	for key := range s.perms {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(s.perms, key)
		}
	}
}

func clonePerms(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *AuthService) GetUserInfo(ctx context.Context, userID string) (AuthUserInfo, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return AuthUserInfo{}, common.ErrNotFound
	}
	if !u.Active {
		return AuthUserInfo{}, common.ErrInactiveUser
	}
	tenantID, err := s.users.GetDefaultTenantID(ctx, u.ID)
	if err != nil {
		return AuthUserInfo{}, err
	}
	roles, _ := s.users.ListUserRoles(ctx, userID, tenantID)
	return AuthUserInfo{ID: u.ID, Email: u.Email, Name: u.Name, TenantID: tenantID, Roles: roles}, nil
}

func (s *AuthService) ensureUserActive(ctx context.Context, userID string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return common.ErrInvalidCredentials
	}
	if !u.Active {
		return common.ErrInactiveUser
	}
	return nil
}

func slowEqual(a, b string) {
	_ = subtle.ConstantTimeCompare([]byte(a), []byte(b))
}
