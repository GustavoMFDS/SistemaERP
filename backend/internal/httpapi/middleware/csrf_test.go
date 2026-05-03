package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/sistemaemgo/internal/config"
)

func TestRequireTrustedOriginAllowsConfiguredOrigin(t *testing.T) {
	cfg := config.Config{Env: "prod", CORSAllowedOrigins: []string{"https://app.example.com"}}
	handler := RequireTrustedOrigin(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected allowed origin, got status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestRequireTrustedOriginRejectsCrossOrigin(t *testing.T) {
	cfg := config.Config{Env: "prod", CORSAllowedOrigins: []string{"https://app.example.com"}}
	handler := RequireTrustedOrigin(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected rejected origin, got status %d", rec.Code)
	}
}

func TestRequireTrustedOriginRequiresOriginInProd(t *testing.T) {
	cfg := config.Config{Env: "prod", CORSAllowedOrigins: []string{"https://app.example.com"}}
	handler := RequireTrustedOrigin(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected missing origin to be rejected in prod, got status %d", rec.Code)
	}
}

func TestRequireTrustedOriginAllowsMissingOriginInDevelopment(t *testing.T) {
	cfg := config.Config{Env: "dev", CORSAllowedOrigins: []string{"http://localhost:5173"}}
	handler := RequireTrustedOrigin(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected missing origin to be allowed in dev, got status %d", rec.Code)
	}
}
