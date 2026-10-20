package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Use the same bcrypt cost when a login identifier has no user. A cheap
// string comparison leaks account existence through response timing.
var unknownUserPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("timing-guard-not-a-user"), bcrypt.DefaultCost)
	if err != nil {
		panic("could not initialize login timing guard")
	}
	return hash
}()

type AuthService struct {
	cfg     config.Config
	users   UsersRepository
	refresh RefreshTokenStore
	logger  *slog.Logger
}

func NewAuthService(cfg config.Config, users UsersRepository, refresh RefreshTokenStore, logger *slog.Logger) *AuthService {
	return &AuthService{cfg: cfg, users: users, refresh: refresh, logger: logger}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (TokenResponse, AuthUserInfo, error) {
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Consume comparable bcrypt work for an unknown identifier.
		_ = bcrypt.CompareHashAndPassword(unknownUserPasswordHash, []byte(password))
		return TokenResponse{}, AuthUserInfo{}, common.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return TokenResponse{}, AuthUserInfo{}, common.ErrInvalidCredentials
	}
	if !u.Active {
		return TokenResponse{}, AuthUserInfo{}, common.ErrInactiveUser
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
	accessTok, accessExp, err := s.issueAccessToken(u.ID, tenantID, u.AuthVersion)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, u.ID, tenantID, u.AuthVersion)
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

func (s *AuthService) issueAccessToken(userID, tenantID string, version int64) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.cfg.AccessTokenTTL)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    s.cfg.JWTIssuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}, Type: "access", TenantID: tenantID, AuthVersion: version}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.JWTSecret))
	return signed, exp, err
}

func (s *AuthService) issueRefreshToken(ctx context.Context, userID, tenantID string, version int64) (string, time.Time, error) {
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
	}, Type: "refresh", TenantID: tenantID, AuthVersion: version}
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
	if err := s.ensureTokenVersion(ctx, claims.Subject, claims.AuthVersion); err != nil {
		return "", "", err
	}
	if err := s.ensureUserTenantAccess(ctx, claims.Subject, claims.TenantID); err != nil {
		return "", "", err
	}
	return claims.Subject, claims.TenantID, nil
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
	if err := s.ensureTokenVersion(ctx, claims.Subject, claims.AuthVersion); err != nil {
		return TokenResponse{}, "", "", err
	}
	if err := s.ensureUserTenantAccess(ctx, claims.Subject, claims.TenantID); err != nil {
		return TokenResponse{}, "", "", err
	}
	accessTok, accessExp, err := s.issueAccessToken(claims.Subject, claims.TenantID, claims.AuthVersion)
	if err != nil {
		return TokenResponse{}, "", "", err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, claims.Subject, claims.TenantID, claims.AuthVersion)
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
	perms, err := s.users.ListUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(perms))
	for _, p := range perms {
		out[p] = true
	}
	return out, nil
}

func (s *AuthService) GetUserInfo(ctx context.Context, userID string, tenantID string) (AuthUserInfo, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return AuthUserInfo{}, common.ErrNotFound
	}
	if !u.Active {
		return AuthUserInfo{}, common.ErrInactiveUser
	}
	if err := s.ensureUserTenantAccess(ctx, userID, tenantID); err != nil {
		return AuthUserInfo{}, err
	}
	roles, err := s.users.ListUserRoles(ctx, userID, tenantID)
	if err != nil {
		return AuthUserInfo{}, err
	}
	permissions, err := s.users.ListUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return AuthUserInfo{}, err
	}
	return AuthUserInfo{
		ID: u.ID, Email: u.Email, Name: u.Name, TenantID: tenantID,
		Roles: roles, Permissions: permissions,
	}, nil
}

func (s *AuthService) ensureTokenVersion(ctx context.Context, userID string, version int64) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return common.ErrInvalidCredentials
	}
	if !u.Active {
		return common.ErrInactiveUser
	}
	if u.AuthVersion != version {
		return common.ErrInvalidCredentials
	}
	return nil
}

func (s *AuthService) ensureUserTenantAccess(ctx context.Context, userID string, tenantID string) error {
	ok, err := s.users.UserHasTenant(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if !ok {
		return common.ErrForbidden
	}
	return nil
}
