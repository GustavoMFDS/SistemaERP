package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/inventory/application"
)

// Query params are bounded to limit=1..50 and offset=0..5000.
// Reject malformed values instead of silently returning a different page.
func readImportHistoryPage(r *http.Request) (int, int, error) {
	limit, offset := 20, 0
	query := r.URL.Query()
	if values, exists := query["limit"]; exists {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, common.ErrValidation
		}
		number, err := strconv.Atoi(values[0])
		if err != nil || number < 1 || number > 50 {
			return 0, 0, common.ErrValidation
		}
		limit = number
	}
	if values, exists := query["offset"]; exists {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, common.ErrValidation
		}
		number, err := strconv.Atoi(values[0])
		if err != nil || number < 0 || number > 5000 {
			return 0, 0, common.ErrValidation
		}
		offset = number
	}
	return limit, offset, nil
}

func writeImportHistory(w http.ResponseWriter, r *http.Request, result application.ImportHistoryPage, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "pagina de historico invalida", nil)
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel consultar o historico", nil)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *ProductsHandler) ListImportHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, offset, err := readImportHistoryPage(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	page, err := h.svc.ListImportHistory(r.Context(), au.TenantID, limit, offset)
	writeImportHistory(w, r, page, err)
}

func (h *InventoryHandler) ListOpeningStockHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, offset, err := readImportHistoryPage(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	page, err := h.svc.ListOpeningStockHistory(r.Context(), au.TenantID, limit, offset)
	writeImportHistory(w, r, page, err)
}
