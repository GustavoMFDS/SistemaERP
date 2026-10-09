package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
	"github.com/go-chi/chi/v5"
)

type FinanceHandler struct {
	svc    *finapp.FinanceService
	logger *slog.Logger
}

func NewFinanceHandler(svc *finapp.FinanceService, logger *slog.Logger) *FinanceHandler {
	return &FinanceHandler{svc: svc, logger: logger}
}

func writeFinanceError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, common.ErrValidation):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, common.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, common.ErrConflict), errors.Is(err, common.ErrInsufficientCash), errors.Is(err, common.ErrCashSessionClosed):
		status = http.StatusConflict
	}
	writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
}

func (h *FinanceHandler) OwnerOverview(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	today := time.Now().Format("2006-01-02")
	if from == "" {
		from = today
	}
	if to == "" {
		to = today
	}
	result, err := h.svc.OwnerOverview(r.Context(), au.TenantID, from, to)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "overview": result})
}

func (h *FinanceHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	data, err := h.svc.Dashboard(r.Context(), au.TenantID, from, to)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "totals": data})
}

func (h *FinanceHandler) ListLedger(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListLedger(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *FinanceHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListPayments(
		r.Context(), au.TenantID,
		r.URL.Query().Get("from"), r.URL.Query().Get("to"),
		r.URL.Query().Get("method"), r.URL.Query().Get("status"),
		limit, offset,
	)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *FinanceHandler) ReconcilePayment(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req finapp.PaymentReconcileRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, status, created, err := h.svc.ReconcilePayment(
		r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"),
		r.Header.Get("Idempotency-Key"), req,
	)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": id, "status": status, "replayed": !created})
}

func (h *FinanceHandler) GetPaymentReconciliationHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	initial, adjustments, err := h.svc.GetPaymentReconciliationHistory(
		r.Context(), au.TenantID, chi.URLParam(r, "id"),
	)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initial":     initial,
		"adjustments": adjustments,
	})
}

func (h *FinanceHandler) AdjustPaymentReconciliation(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req finapp.PaymentReconciliationAdjustmentRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, status, created, err := h.svc.AdjustPaymentReconciliation(
		r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"),
		r.Header.Get("Idempotency-Key"), req,
	)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": id, "status": status, "replayed": !created})
}

func (h *FinanceHandler) ListRefunds(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListReturnRefunds(r.Context(), au.TenantID, r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *FinanceHandler) SettleRefund(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req finapp.ReturnRefundRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, status, remaining, created, err := h.svc.SettleReturnRefund(
		r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"),
		r.Header.Get("Idempotency-Key"), req,
	)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{
		"id": id, "status": status, "remaining_amount": remaining, "replayed": !created,
	})
}
