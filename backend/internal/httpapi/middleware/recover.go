package middleware

import (
	"log/slog"
	"net/http"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/go-chi/chi/v5/middleware"
)

func Recover(cfg config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					attrs := []slog.Attr{
						slog.String("request_id", middleware.GetReqID(r.Context())),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
					}
					if cfg.IsProdLike() {
						logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered", attrs...)
					} else {
						attrs = append(attrs, slog.Any("recover", rec))
						logger.LogAttrs(r.Context(), slog.LevelError, "panic_recovered", attrs...)
					}
					writeMiddlewareError(w, r, http.StatusInternalServerError, "internal_error", "erro interno")
				}
			}()
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
