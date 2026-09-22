package handlers

import (
	"errors"
	"net/http"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	"github.com/go-chi/chi/v5"
)

type CashHandler struct {
	svc    *salesapp.CashService
	logger *slog.Logger
}

func NewCashHandler(svc *salesapp.CashService, logger *slog.Logger) *CashHandler {
	return &CashHandler{svc: svc, logger: logger}
}

func (h *CashHandler) OpenSession(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req salesapp.CashOpenRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, err := h.svc.OpenSession(r.Context(), au.TenantID, au.UserID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, common.ErrValidation):
			status = http.StatusUnprocessableEntity
		case errors.Is(err, common.ErrCashSessionAlreadyOpen):
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *CashHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	sessionID := chi.URLParam(r, "id")
	var req salesapp.CashCloseRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.CloseSession(r.Context(), au.TenantID, au.UserID, sessionID, req); err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, common.ErrValidation):
			status = http.StatusUnprocessableEntity
		case errors.Is(err, common.ErrCashSessionClosed):
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
