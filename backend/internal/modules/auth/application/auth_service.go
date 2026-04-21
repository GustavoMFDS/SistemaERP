package application

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

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

	roles, _ := s.users.ListUserRoles(ctx, u.ID)
	accessTok, accessExp, err := s.issueAccessToken(u.ID)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, u.ID)
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
			ID: u.ID, Email: u.Email, Name: u.Name, Roles: roles,
		}, nil
}

func (s *AuthService) issueAccessToken(userID string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.cfg.AccessTokenTTL)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    s.cfg.JWTIssuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}, Type: "access"}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.JWTSecret))
	return signed, exp, err
}

func (s *AuthService) issueRefreshToken(ctx context.Context, userID string) (string, time.Time, error) {
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
	}, Type: "refresh"}
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

func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (string, error) {
	_ = ctx
	parsed, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return "", err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return "", common.ErrInvalidCredentials
	}
	if claims.Issuer != s.cfg.JWTIssuer {
		return "", common.ErrInvalidCredentials
	}
	if claims.Type != "access" {
		return "", common.ErrInvalidCredentials
	}
	return claims.Subject, nil
}

func (s *AuthService) Refresh(ctx context.Context, tokenStr string) (TokenResponse, error) {
	claims, err := s.validateRefreshToken(tokenStr)
	if err != nil {
		return TokenResponse{}, err
	}
	if s.refresh == nil {
		return TokenResponse{}, common.ErrInvalidCredentials
	}
	ok, err := s.refresh.Consume(ctx, claims.ID, claims.Subject)
	if err != nil {
		return TokenResponse{}, err
	}
	if !ok {
		return TokenResponse{}, common.ErrInvalidCredentials
	}
	accessTok, accessExp, err := s.issueAccessToken(claims.Subject)
	if err != nil {
		return TokenResponse{}, err
	}
	refreshTok, refreshExp, err := s.issueRefreshToken(ctx, claims.Subject)
	if err != nil {
		return TokenResponse{}, err
	}
	return TokenResponse{
		AccessToken:      accessTok,
		RefreshToken:     refreshTok,
		TokenType:        "Bearer",
		ExpiresIn:        int64(time.Until(accessExp).Seconds()),
		RefreshExpiresIn: int64(time.Until(refreshExp).Seconds()),
	}, nil
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
	if claims.ID == "" || claims.Subject == "" {
		return nil, common.ErrInvalidCredentials
	}
	return claims, nil
}

func (s *AuthService) GetUserPermissions(ctx context.Context, userID string) (map[string]bool, error) {
	perms, err := s.users.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(perms))
	for _, p := range perms {
		m[p] = true
	}
	return m, nil
}

func (s *AuthService) GetUserInfo(ctx context.Context, userID string) (AuthUserInfo, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return AuthUserInfo{}, common.ErrNotFound
	}
	roles, _ := s.users.ListUserRoles(ctx, userID)
	return AuthUserInfo{ID: u.ID, Email: u.Email, Name: u.Name, Roles: roles}, nil
}

func slowEqual(a, b string) {
	_ = subtle.ConstantTimeCompare([]byte(a), []byte(b))
}
