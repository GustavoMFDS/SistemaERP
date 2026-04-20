package application

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
)

type AuthService struct {
	cfg    config.Config
	users  UsersRepository
	logger *slog.Logger
}

func NewAuthService(cfg config.Config, users UsersRepository, logger *slog.Logger) *AuthService {
	return &AuthService{cfg: cfg, users: users, logger: logger}
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
	tok, exp, err := s.issueToken(u.ID)
	if err != nil {
		return TokenResponse{}, AuthUserInfo{}, err
	}
	return TokenResponse{AccessToken: tok, TokenType: "Bearer", ExpiresIn: int64(time.Until(exp).Seconds())}, AuthUserInfo{
		ID: u.ID, Email: u.Email, Name: u.Name, Roles: roles,
	}, nil
}

func (s *AuthService) issueToken(userID string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(s.cfg.JWTTTL)
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer:    s.cfg.JWTIssuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(s.cfg.JWTSecret))
	return signed, exp, err
}

func (s *AuthService) ValidateToken(ctx context.Context, tokenStr string) (string, error) {
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
	return claims.Subject, nil
}

func (s *AuthService) Refresh(ctx context.Context, tokenStr string) (TokenResponse, error) {
	userID, err := s.ValidateToken(ctx, tokenStr)
	if err != nil {
		return TokenResponse{}, err
	}
	tok, exp, err := s.issueToken(userID)
	if err != nil {
		return TokenResponse{}, err
	}
	return TokenResponse{AccessToken: tok, TokenType: "Bearer", ExpiresIn: int64(time.Until(exp).Seconds())}, nil
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
