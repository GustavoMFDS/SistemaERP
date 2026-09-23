package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	retapp "github.com/example/sistemaemgo/internal/modules/returns/application"
	"github.com/go-chi/chi/v5"
)

type ReturnsHandler struct {
	svc    *retapp.Service
	audit  *audit.Service
	logger *slog.Logger
}

func NewReturnsHandler(svc *retapp.Service, auditSvc *audit.Service, logger *slog.Logger) *ReturnsHandler {
	return &ReturnsHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *ReturnsHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListReturns(r.Context(), au.TenantID, chi.URLParam(r, "id"), limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro interno", nil)
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
	value, items, refunds, err := h.svc.GetReturn(r.Context(), au.TenantID, chi.URLParam(r, "returnID"))
	if err != nil || value.SaleID != chi.URLParam(r, "id") {
		writeError(w, r, http.StatusNotFound, "not_found", "devolucao nao encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"return": value, "items": items, "refunds": refunds})
}

func (h *ReturnsHandler) Create(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req retapp.CreateReturnRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	value, created, err := h.svc.CreateReturn(
		r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"),
		r.Header.Get("Idempotency-Key"), req,
	)
	if err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	if created {
		recordAudit(h.audit, r, au.TenantID, au.UserID, "sale.return.create", "sale_return", value.ID, "success", map[string]any{
			"sale_id": value.SaleID, "total_amount": value.TotalAmount.String(),
		})
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"return": value, "replayed": !created})
}

func (h *ReturnsHandler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req retapp.CreateRefundRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	value, created, err := h.svc.CreateRefund(
		r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"), chi.URLParam(r, "returnID"),
		r.Header.Get("Idempotency-Key"), req,
	)
	if err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	if created {
		recordAudit(h.audit, r, au.TenantID, au.UserID, "sale.return.refund", "sale_refund", value.ID, "success", map[string]any{
			"sale_id": value.SaleID, "return_id": value.ReturnID, "method": value.Method, "amount": value.Amount.String(),
		})
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"refund": value, "replayed": !created})
}
