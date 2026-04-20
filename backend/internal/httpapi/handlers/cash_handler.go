package handlers

import (
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

type CashHandler struct {
	svc    *salesapp.CashService
	logger *slog.Logger
	pool   *pgxpool.Pool
}

func NewCashHandler(svc *salesapp.CashService, logger *slog.Logger) *CashHandler {
	return &CashHandler{svc: svc, logger: logger}
}

func (h *CashHandler) BindDB(pool *pgxpool.Pool) {
	h.pool = pool
}

func (h *CashHandler) OpenSession(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "db not configured", http.StatusInternalServerError)
		return
	}
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req salesapp.CashOpenRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	id, err := h.svc.OpenSession(r.Context(), h.pool, au.UserID, req)
	if err != nil {
		status := http.StatusBadRequest
		if err == common.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *CashHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "db not configured", http.StatusInternalServerError)
		return
	}
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "id")
	var req salesapp.CashCloseRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.svc.CloseSession(r.Context(), h.pool, au.UserID, sessionID, req); err != nil {
		status := http.StatusBadRequest
		if err == common.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
