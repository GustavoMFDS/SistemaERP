package handlers

import (
	"net/http"
	"strconv"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	"github.com/go-chi/chi/v5"
)

type SalesHandler struct {
	svc    *salesapp.SalesService
	audit  *audit.Service
	logger *slog.Logger
}

func NewSalesHandler(svc *salesapp.SalesService, auditSvc *audit.Service, logger *slog.Logger) *SalesHandler {
	return &SalesHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *SalesHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.List(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar vendas", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *SalesHandler) Get(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	sale, items, pays, err := h.svc.Get(r.Context(), au.TenantID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sale": sale, "items": items, "payments": pays})
}

func (h *SalesHandler) CreateAndFinalize(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	var req salesapp.SaleCreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	cashSessionID, valid := normalizeUUID(req.CashSessionID)
	if !valid {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "cash_session_id invalido", nil)
		return
	}
	req.CashSessionID = cashSessionID
	if req.CustomerID != nil {
		customerID, valid := normalizeUUID(*req.CustomerID)
		if !valid {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "customer_id invalido", nil)
			return
		}
		req.CustomerID = &customerID
	}
	for i := range req.Items {
		productID, valid := normalizeUUID(req.Items[i].ProductID)
		if !valid {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "product_id invalido", nil)
			return
		}
		req.Items[i].ProductID = productID
	}

	saleID, total, created, err := h.svc.CreateAndFinalize(r.Context(), au.TenantID, au.UserID, idemKey, req)
	if err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrInsufficientStock:
			status = http.StatusConflict
		case common.ErrCashSessionClosed:
			status = http.StatusConflict
		case common.ErrPaymentsMismatch:
			status = http.StatusConflict
		case common.ErrPriceChanged:
			status = http.StatusConflict
		case common.ErrConflict:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	code := http.StatusCreated
	if !created {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]any{"id": saleID, "status": "finalized", "total": total, "replayed": !created})
}

func (h *SalesHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	saleID, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req salesapp.SaleCancelRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.Cancel(r.Context(), au.TenantID, au.UserID, saleID, req); err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrSaleNotFinalized, common.ErrSaleAlreadyCancelled, common.ErrCashSessionClosed, common.ErrConflict:
			status = http.StatusConflict
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}
