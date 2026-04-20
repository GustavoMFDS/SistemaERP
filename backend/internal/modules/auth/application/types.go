package application

import "github.com/golang-jwt/jwt/v5"

type AuthUserInfo struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type Claims struct {
	jwt.RegisteredClaims
}
