package middleware

import (
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
)

// CORS enables a minimal CORS policy for local development.
//
// Rationale: the frontend (Vite) runs on a different origin (e.g. http://localhost:5173),
// so the browser sends a preflight OPTIONS request which must be answered.
//
// Policy:
// - Enabled only when APP_ENV is not prod-like.
// - Echoes Origin (no wildcard) and sets Vary: Origin.
// - Allows common methods and headers used by this API.
// - Does not set Access-Control-Allow-Credentials (tokens are sent via Authorization header).
func CORS(cfg config.Config) func(http.Handler) http.Handler {
	enabled := !cfg.IsProdLike()

	allowedMethods := strings.Join([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, ", ")
	allowedHeaders := strings.Join([]string{"Authorization", "Content-Type", "Accept", "Idempotency-Key"}, ", ")
	exposedHeaders := strings.Join([]string{"Content-Type"}, ", ")

	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Expose-Headers", exposedHeaders)
				w.Header().Set("Access-Control-Max-Age", "600")
			}

			// Handle preflight.
			if r.Method == http.MethodOptions {
				// If this is a CORS preflight request, respond without hitting downstream routes.
				if origin != "" && r.Header.Get("Access-Control-Request-Method") != "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				// Non-CORS OPTIONS: let router decide.
			}

			next.ServeHTTP(w, r)
		})
	}
}
