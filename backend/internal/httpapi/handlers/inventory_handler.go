package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	"github.com/go-chi/chi/v5"
)

type InventoryHandler struct {
	svc    *invapp.InventoryService
	logger *slog.Logger
}

func NewInventoryHandler(svc *invapp.InventoryService, logger *slog.Logger) *InventoryHandler {
	return &InventoryHandler{svc: svc, logger: logger}
}

func (h *InventoryHandler) LowStock(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.LowStock(r.Context(), au.TenantID, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar estoque", nil)
		return
	}
	total, err := h.svc.LowStockCount(r.Context(), au.TenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao contar estoque baixo", nil)
		return
	}
	if !middleware.HasPermission(r.Context(), "finance:read") {
		for i := range items {
			items[i].CostPrice = 0
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *InventoryHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	productID := r.URL.Query().Get("product_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListMovements(r.Context(), au.TenantID, productID, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar movimentacoes", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// OpeningStockBatch returns only a committed batch in this authenticated tenant.
func (h *InventoryHandler) OpeningStockBatch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	result, found, err := h.svc.LookupOpeningStockBatch(r.Context(), au.TenantID, chi.URLParam(r, "key"))
	if err == common.ErrValidation {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "referencia de lote invalida", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar lote", nil)
		return
	}
	if !found {
		writeError(w, r, http.StatusNotFound, "not_found", "lote nao confirmado nesta empresa", nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ImportOpeningStock applies one explicitly confirmed opening count only once.
func (h *InventoryHandler) ImportOpeningStock(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 128 {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error",
			"Informe uma chave de idempotencia entre 8 e 128 caracteres.", nil)
		return
	}
	var req invapp.OpeningStockRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	result, err := h.svc.ImportOpeningStock(r.Context(), au.TenantID, au.UserID, key, req)
	if err != nil {
		status := http.StatusInternalServerError
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
	if result.Replayed {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *InventoryHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req invapp.InventoryAdjustRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.Adjust(r.Context(), au.TenantID, au.UserID, req); err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrInsufficientStock:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
