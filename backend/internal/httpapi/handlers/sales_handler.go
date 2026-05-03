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
	id := chi.URLParam(r, "id")
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
	if created {
		requestID, ip, userAgent := audit.RequestContext(r)
		h.audit.Record(r.Context(), audit.Event{
			TenantID:     au.TenantID,
			ActorUserID:  au.UserID,
			Action:       "sale.create",
			ResourceType: "sale",
			ResourceID:   saleID,
			Outcome:      "success",
			Metadata:     map[string]any{"total": total.String()},
			RequestID:    requestID,
			IP:           ip,
			UserAgent:    userAgent,
		})
	}
	writeJSON(w, code, map[string]any{"id": saleID, "status": "finalized", "total": total, "replayed": !created})
}

func (h *SalesHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	saleID := chi.URLParam(r, "id")
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
		case common.ErrSaleNotFinalized, common.ErrSaleAlreadyCancelled:
			status = http.StatusConflict
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	requestID, ip, userAgent := audit.RequestContext(r)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     au.TenantID,
		ActorUserID:  au.UserID,
		Action:       "sale.cancel",
		ResourceType: "sale",
		ResourceID:   saleID,
		Outcome:      "success",
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}
