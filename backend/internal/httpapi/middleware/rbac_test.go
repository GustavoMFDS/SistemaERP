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

func TestRequireAnyPermissionAllowsOnlyAuthorizedRoles(t *testing.T) {
	protected := RequireAnyPermission("product:write", "inventory:adjust")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	for _, tc := range []struct {
		name string
		perms map[string]bool
		expected int
	}{
		{name: "products only", perms: map[string]bool{"product:write": true}, expected: http.StatusNoContent},
		{name: "stock only", perms: map[string]bool{"inventory:adjust": true}, expected: http.StatusNoContent},
		{name: "both", perms: map[string]bool{"product:write": true, "inventory:adjust": true}, expected: http.StatusNoContent},
		{name: "cashier", perms: map[string]bool{"sale:write": true}, expected: http.StatusForbidden},
		{name: "read only", perms: map[string]bool{"product:read": true, "inventory:read": true}, expected: http.StatusForbidden},
		{name: "missing permissions", expected: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/imports/history", nil)
			if tc.perms != nil {
				r = r.WithContext(context.WithValue(r.Context(), permsKey, tc.perms))
			}
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, r)
			if w.Code != tc.expected {
				t.Fatalf("expected status %d, got %d: %s", tc.expected, w.Code, w.Body.String())
			}
		})
	}
}
