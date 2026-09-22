package middleware

import (
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
)

func CORS(cfg config.Config) func(http.Handler) http.Handler {
	allowedOrigins := make(map[string]struct{}, len(cfg.CORSAllowedOrigins))
	for _, o := range cfg.CORSAllowedOrigins {
		allowedOrigins[strings.TrimSpace(o)] = struct{}{}
	}
	allowedMethods := strings.Join(cfg.CORSAllowedMethods, ", ")
	allowedHeaders := strings.Join(cfg.CORSAllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				if _, ok := allowedOrigins[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
					w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
					w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
					w.Header().Set("Access-Control-Max-Age", "600")
				} else if cfg.IsProdLike() {
					writeMiddlewareError(w, r, http.StatusForbidden, "cors_origin_denied", "origin not allowed")
					return
				}
			}

			if r.Method == http.MethodOptions && origin != "" && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
