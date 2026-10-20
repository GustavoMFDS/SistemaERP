package handlers

import (
	"errors"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
)

// ListUnifiedImportHistory shows only the streams permitted to this exact
// authenticated user and company. Permissions are reloaded on each request.
func (h *ProductsHandler) ListUnifiedImportHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	canProducts := middleware.HasPermission(r.Context(), "product:write")
	canOpeningStock := middleware.HasPermission(r.Context(), "inventory:adjust")
	if !canProducts && !canOpeningStock {
		writeError(w, r, http.StatusForbidden, "authorization_error", "permissao insuficiente", nil)
		return
	}
	limit, offset, err := readImportHistoryPage(r)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "pagina de historico invalida", nil)
		return
	}
	filter, err := readImportHistoryFilter(r)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "periodo de historico invalido", nil)
		return
	}
	page, err := h.svc.ListUnifiedImportHistory(
		r.Context(), au.TenantID, canProducts, canOpeningStock, limit, offset, filter,
	)
	switch {
	case errors.Is(err, common.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "authorization_error", "permissao insuficiente", nil)
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "filtros invalidos", nil)
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar historico unificado", nil)
	default:
		writeJSON(w, http.StatusOK, page)
	}
}
