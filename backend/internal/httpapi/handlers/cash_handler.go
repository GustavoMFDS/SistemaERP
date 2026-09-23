package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	"github.com/go-chi/chi/v5"
)

type CashHandler struct {
	svc    *salesapp.CashService
	audit  *audit.Service
	logger *slog.Logger
}

func NewCashHandler(svc *salesapp.CashService, auditSvc *audit.Service, logger *slog.Logger) *CashHandler {
	return &CashHandler{svc: svc, audit: auditSvc, logger: logger}
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

func (h *CashHandler) RecordMovement(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	sessionID := chi.URLParam(r, "id")
	var req salesapp.CashMovementRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, err := h.svc.RecordMovement(r.Context(), au.TenantID, au.UserID, sessionID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, common.ErrValidation):
			status = http.StatusUnprocessableEntity
		case errors.Is(err, common.ErrCashSessionClosed), errors.Is(err, common.ErrInsufficientCash):
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "recorded"})
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
	result, err := h.svc.CloseSession(r.Context(), au.TenantID, au.UserID, sessionID, req)
	if err != nil {
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
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "closed",
		"expected_cash":        result.ExpectedCash,
		"closing_amount":       result.ClosingAmount,
		"closing_difference":   result.ClosingDifference,
		"expected_by_method":   result.ExpectedByMethod,
		"declared_by_method":   result.DeclaredByMethod,
		"difference_by_method": result.DifferenceByMethod,
	})
}
