package application

import "github.com/golang-jwt/jwt/v5"

type AuthUserInfo struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	TenantID    string   `json:"tenant_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in,omitempty"`
}

type Claims struct {
	jwt.RegisteredClaims
	Type     string `json:"typ,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
}
