package middleware

import (
	"context"
	"log/slog"
	"net/http"

	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	"github.com/go-chi/chi/v5/middleware"
)

type ctxPermKey string

const permsKey ctxPermKey = "permissions"

func LoadPermissions(auth *authapp.AuthService, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			au, ok := GetAuthUser(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			perms, err := auth.GetUserPermissions(r.Context(), au.UserID)
			if err != nil {
				logger.Error("load_permissions_failed", slog.Any("err", err), slog.String("request_id", middleware.GetReqID(r.Context())))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), permsKey, perms)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			perms, _ := r.Context().Value(permsKey).(map[string]bool)
			if perms == nil || !perms[perm] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
