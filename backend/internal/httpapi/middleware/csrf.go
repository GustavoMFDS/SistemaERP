package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
)

func RequireTrustedOrigin(cfg config.Config) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.CORSAllowedOrigins))
	for _, origin := range cfg.CORSAllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" {
				if _, ok := allowed[origin]; ok {
					next.ServeHTTP(w, r)
					return
				}
				writeMiddlewareError(w, r, http.StatusForbidden, "csrf_origin_denied", "origin not allowed")
				return
			}

			referer := strings.TrimSpace(r.Header.Get("Referer"))
			if referer != "" {
				refOrigin, err := originFromReferer(referer)
				if err == nil {
					if _, ok := allowed[refOrigin]; ok {
						next.ServeHTTP(w, r)
						return
					}
				}
				writeMiddlewareError(w, r, http.StatusForbidden, "csrf_origin_denied", "referer not allowed")
				return
			}

			if cfg.IsProdLike() {
				writeMiddlewareError(w, r, http.StatusForbidden, "csrf_origin_required", "origin required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originFromReferer(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}
