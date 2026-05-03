package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
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
