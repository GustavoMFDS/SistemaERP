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
				writeMiddlewareError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado")
				return
			}
			perms, err := auth.GetUserPermissions(r.Context(), au.UserID, au.TenantID)
			if err != nil {
				logger.Error("load_permissions_failed", slog.Any("err", err), slog.String("request_id", middleware.GetReqID(r.Context())))
				writeMiddlewareError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado")
				return
			}
			ctx := context.WithValue(r.Context(), permsKey, perms)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func HasPermission(ctx context.Context, perm string) bool {
	perms, _ := ctx.Value(permsKey).(map[string]bool)
	return perms != nil && perms[perm]
}

func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			perms, _ := r.Context().Value(permsKey).(map[string]bool)
			if perms == nil || !perms[perm] {
				writeMiddlewareError(w, r, http.StatusForbidden, "authorization_error", "permissao insuficiente")
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
