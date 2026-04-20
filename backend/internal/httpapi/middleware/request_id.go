package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

func RequestID() func(http.Handler) http.Handler {
	return middleware.RequestID
}

func GetRequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	if v == "" {
		// chi middleware stores under middleware.RequestIDKey
		if vv, ok := ctx.Value(middleware.RequestIDKey).(string); ok {
			return vv
		}
	}
	return v
}
