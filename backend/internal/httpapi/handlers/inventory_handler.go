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
	"github.com/google/uuid"
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

// readMovementsQuery bounds pagination and rejects malformed product IDs,
// duplicate parameters and tenant selectors before reaching PostgreSQL.
func readMovementsQuery(r *http.Request) (productID string, limit, offset int, err error) {
	query := r.URL.Query()
	for key, values := range query {
		if (key != "product_id" && key != "limit" && key != "offset") || len(values) != 1 {
			return "", 0, 0, common.ErrValidation
		}
	}
	productID = query.Get("product_id")
	if productID != "" {
		if _, parseErr := uuid.Parse(productID); parseErr != nil {
			return "", 0, 0, common.ErrValidation
		}
	}
	limit, offset = 100, 0
	if _, present := query["limit"]; present {
		parsed, parseErr := strconv.Atoi(query.Get("limit"))
		if parseErr != nil || parsed < 1 || parsed > 500 {
			return "", 0, 0, common.ErrValidation
		}
		limit = parsed
	}
	if _, present := query["offset"]; present {
		parsed, parseErr := strconv.Atoi(query.Get("offset"))
		if parseErr != nil || parsed < 0 || parsed > 5000 {
			return "", 0, 0, common.ErrValidation
		}
		offset = parsed
	}
	return productID, limit, offset, nil
}

func (h *InventoryHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	productID, limit, offset, err := readMovementsQuery(r)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "consulta de movimentacoes invalida", nil)
		return
	}
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
