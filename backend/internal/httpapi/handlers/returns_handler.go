package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	retapp "github.com/example/sistemaemgo/internal/modules/returns/application"
	"github.com/go-chi/chi/v5"
)

type ReturnsHandler struct {
	svc    *retapp.Service
	logger *slog.Logger
}

func NewReturnsHandler(svc *retapp.Service, logger *slog.Logger) *ReturnsHandler {
	return &ReturnsHandler{svc: svc, logger: logger}
}

func writeReturnsError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, common.ErrValidation):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, common.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, common.ErrConflict):
		status = http.StatusConflict
	}
	writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
}

func (h *ReturnsHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.List(r.Context(), au.TenantID, r.URL.Query().Get("sale_id"), limit, offset)
	if err != nil {
		writeReturnsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *ReturnsHandler) Get(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	item, items, err := h.svc.Get(r.Context(), au.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		writeReturnsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"return": item, "items": items})
}

func (h *ReturnsHandler) CreateForSale(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req retapp.CreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, refundDue, created, err := h.svc.Create(
		r.Context(),
		au.TenantID,
		au.UserID,
		chi.URLParam(r, "id"),
		r.Header.Get("Idempotency-Key"),
		req,
	)
	if err != nil {
		writeReturnsError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{
		"id": id,
		"refund_due": refundDue,
		"refund_status": "pending",
		"replayed": !created,
	})
}
