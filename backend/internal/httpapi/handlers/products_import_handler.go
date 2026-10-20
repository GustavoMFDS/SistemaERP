package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	"github.com/go-chi/chi/v5"
)

func (h *ProductsHandler) ImportBatch(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 128 {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "referencia de lote invalida", nil)
		return
	}
	var req invapp.ProductImportRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if !middleware.HasPermission(r.Context(), "finance:read") {
		for i := range req.Items {
			req.Items[i].CostPrice = 0
		}
	}
	result, err := h.svc.ImportProducts(r.Context(), au.TenantID, au.UserID, key, req)
	if err != nil {
		switch {
		case errors.Is(err, common.ErrValidation):
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", friendlyErrorMessage(err), nil)
		case errors.Is(err, common.ErrConflict):
			writeError(w, r, http.StatusConflict, "conflict", "o lote conflita com produtos existentes ou com uma tentativa diferente", nil)
		default:
			writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel importar os produtos", nil)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if result.Replayed {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// A GET only confirms a committed receipt; a 404 cannot rule out an
// in-flight request. The original key + payload are required for a safe replay.
func (h *ProductsHandler) GetImportBatch(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	result, found, err := h.svc.LookupProductImportBatch(r.Context(), au.TenantID, chi.URLParam(r, "key"))
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "referencia de lote invalida", nil)
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel consultar o lote", nil)
	case !found:
		writeError(w, r, http.StatusNotFound, "not_found", "nenhum lote confirmado com essa referencia nesta empresa", nil)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}
