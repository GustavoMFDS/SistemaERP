package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	"github.com/go-chi/chi/v5/middleware"
)

type ctxUserKey string

type AuthUser struct {
	UserID string
}

const userKey ctxUserKey = "auth_user"

func AuthJWT(cfg config.Config, auth *authapp.AuthService, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimSpace(h[len("Bearer "):])
			userID, err := auth.ValidateToken(r.Context(), token)
			if err != nil {
				logger.Warn("auth_invalid_token", slog.Any("err", err), slog.String("request_id", middleware.GetReqID(r.Context())))
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, AuthUser{UserID: userID})
			next.ServeHTTP(w, r.WithContext(ctx))
		}
		return http.HandlerFunc(fn)
	}
}

func GetAuthUser(ctx context.Context) (AuthUser, bool) {
	v, ok := ctx.Value(userKey).(AuthUser)
	return v, ok
}
