package handlers

import (
	"net/http"
	"strconv"
	"time"

	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
	"log/slog"
)

type FinanceHandler struct {
	svc    *finapp.FinanceService
	logger *slog.Logger
}

func NewFinanceHandler(svc *finapp.FinanceService, logger *slog.Logger) *FinanceHandler {
	return &FinanceHandler{svc: svc, logger: logger}
}

func (h *FinanceHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = time.Now().Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	data, err := h.svc.Dashboard(r.Context(), from, to)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "totals": data})
}

func (h *FinanceHandler) ListLedger(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListLedger(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}
