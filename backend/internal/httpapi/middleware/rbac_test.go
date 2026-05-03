package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequirePermissionAllowsAndDenies(t *testing.T) {
	protected := RequirePermission("audit:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowedReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs", nil)
	allowedReq = allowedReq.WithContext(context.WithValue(allowedReq.Context(), permsKey, map[string]bool{"audit:read": true}))
	allowedRec := httptest.NewRecorder()
	protected.ServeHTTP(allowedRec, allowedReq)
	if allowedRec.Code != http.StatusNoContent {
		t.Fatalf("expected permission to allow request, got %d", allowedRec.Code)
	}

	deniedReq := httptest.NewRequest(http.MethodGet, "/api/v1/audit/logs", nil)
	deniedReq = deniedReq.WithContext(context.WithValue(deniedReq.Context(), permsKey, map[string]bool{"privacy:read": true}))
	deniedRec := httptest.NewRecorder()
	protected.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected permission denial, got %d", deniedRec.Code)
	}
}
