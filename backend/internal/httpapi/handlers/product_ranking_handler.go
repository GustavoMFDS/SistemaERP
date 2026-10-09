package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
)

func (h *FinanceHandler) ProductRanking(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	query := r.URL.Query()
	for name, values := range query {
		if (name != "from" && name != "to" && name != "limit") || len(values) != 1 {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "parametro de ranking invalido", nil)
			return
		}
	}
	for _, name := range []string{"from", "to"} {
		if values := query[name]; len(values) != 1 || values[0] == "" {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "informe o periodo", nil)
			return
		}
	}
	limit := 10
	if values, present := query["limit"]; present {
		if len(values) != 1 {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "limite invalido", nil)
			return
		}
		var err error
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 50 {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "limite invalido", nil)
			return
		}
	}
	items, err := h.svc.ProductRanking(r.Context(), au.TenantID,
		query.Get("from"), query.Get("to"), limit)
	if err != nil {
		writeFinanceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items,
		"from": query.Get("from"), "to": query.Get("to"), "limit": limit})
}
