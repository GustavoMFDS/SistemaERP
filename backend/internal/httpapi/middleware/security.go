package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/example/sistemaemgo/internal/config"
)

func SecurityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			if cfg.IsProdLike() {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ProtectMetrics(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !cfg.IsProdLike() && cfg.MetricsBearerToken == "" && (cfg.MetricsBasicUser == "" || cfg.MetricsBasicPass == "") {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.MetricsBearerToken != "" {
				got := r.Header.Get("Authorization")
				want := "Bearer " + cfg.MetricsBearerToken
				if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			if cfg.MetricsBasicUser != "" && cfg.MetricsBasicPass != "" {
				user, pass, ok := r.BasicAuth()
				if ok &&
					subtle.ConstantTimeCompare([]byte(user), []byte(cfg.MetricsBasicUser)) == 1 &&
					subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.MetricsBasicPass)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
			}
			writeMiddlewareError(w, r, http.StatusUnauthorized, "authentication_error", "metrics authentication required")
		})
	}
}

func writeMiddlewareError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":       code,
		"message":    message,
		"request_id": GetRequestID(r.Context()),
	})
}
