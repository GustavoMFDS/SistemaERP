package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	"github.com/go-chi/chi/v5"
	"log/slog"
)

type SalesHandler struct {
	svc    *salesapp.SalesService
	logger *slog.Logger
}

func NewSalesHandler(svc *salesapp.SalesService, logger *slog.Logger) *SalesHandler {
	return &SalesHandler{svc: svc, logger: logger}
}

func (h *SalesHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *SalesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sale, items, pays, err := h.svc.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sale": sale, "items": items, "payments": pays})
}

func (h *SalesHandler) CreateAndFinalize(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req salesapp.SaleCreateRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	saleID, total, err := h.svc.CreateAndFinalize(r.Context(), au.UserID, req)
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
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": saleID, "status": "finalized", "total": total})
}

func (h *SalesHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	saleID := chi.URLParam(r, "id")
	var req salesapp.SaleCancelRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.svc.Cancel(r.Context(), au.UserID, saleID, req); err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrSaleNotFinalized, common.ErrSaleAlreadyCancelled:
			status = http.StatusConflict
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}
