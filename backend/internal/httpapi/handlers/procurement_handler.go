package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	procapp "github.com/example/sistemaemgo/internal/modules/procurement/application"
	"github.com/go-chi/chi/v5"
)

type ProcurementHandler struct {
	svc    *procapp.Service
	audit  *audit.Service
	logger *slog.Logger
}

func NewProcurementHandler(svc *procapp.Service, auditSvc *audit.Service, logger *slog.Logger) *ProcurementHandler {
	return &ProcurementHandler{svc: svc, audit: auditSvc, logger: logger}
}

func writeProcurementError(w http.ResponseWriter, r *http.Request, err error) {
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

func (h *ProcurementHandler) ListSuppliers(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListSuppliers(r.Context(), au.TenantID, r.URL.Query().Get("query"), limit, offset)
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *ProcurementHandler) CreateSupplier(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	var req procapp.SupplierRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, created, err := h.svc.CreateSupplier(r.Context(), au.TenantID, au.UserID, idemKey, req)
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": id, "replayed": !created})
}

func (h *ProcurementHandler) UpdateSupplier(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id := chi.URLParam(r, "id")
	var req procapp.SupplierRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.UpdateSupplier(r.Context(), au.TenantID, au.UserID, id, req); err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *ProcurementHandler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListPurchases(r.Context(), au.TenantID, r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *ProcurementHandler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	purchase, items, receipts, err := h.svc.GetPurchase(r.Context(), au.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"purchase": purchase, "items": items, "receipts": receipts})
}

func (h *ProcurementHandler) CreatePurchase(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	var req procapp.PurchaseCreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, created, err := h.svc.CreatePurchase(r.Context(), au.TenantID, au.UserID, idemKey, req)
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": id, "status": "ordered", "replayed": !created})
}

func (h *ProcurementHandler) ReceivePurchase(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	purchaseID := chi.URLParam(r, "id")
	idemKey := r.Header.Get("Idempotency-Key")
	var req procapp.PurchaseReceiveRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	receiptID, status, created, err := h.svc.ReceivePurchase(r.Context(), au.TenantID, au.UserID, purchaseID, idemKey, req)
	if err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipt_id": receiptID, "status": status, "replayed": !created})
}

func (h *ProcurementHandler) CancelPurchase(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	purchaseID := chi.URLParam(r, "id")
	if err := h.svc.CancelPurchase(r.Context(), au.TenantID, au.UserID, purchaseID); err != nil {
		writeProcurementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}
