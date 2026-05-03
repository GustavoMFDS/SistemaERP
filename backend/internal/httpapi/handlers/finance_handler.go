package handlers

import (
	"net/http"
	"strconv"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
)

type FinanceHandler struct {
	svc    *finapp.FinanceService
	logger *slog.Logger
}

func NewFinanceHandler(svc *finapp.FinanceService, logger *slog.Logger) *FinanceHandler {
	return &FinanceHandler{svc: svc, logger: logger}
}

func (h *FinanceHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	data, err := h.svc.Dashboard(r.Context(), au.TenantID, from, to)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao carregar dashboard", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "totals": data})
}

func (h *FinanceHandler) ListLedger(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListLedger(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar lancamentos", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}
