package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

type InventoryHandler struct {
	svc    *service.InventoryService
	logger *slog.Logger
	pool   *pgxpool.Pool
}

func NewInventoryHandler(svc *service.InventoryService, logger *slog.Logger) *InventoryHandler {
	return &InventoryHandler{svc: svc, logger: logger}
}

func (h *InventoryHandler) BindDB(pool *pgxpool.Pool) {
	h.pool = pool
}

func (h *InventoryHandler) LowStock(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.svc.LowStock(r.Context(), limit)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *InventoryHandler) ListMovements(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("product_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListMovements(r.Context(), productID, limit, offset)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *InventoryHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "db not configured", http.StatusInternalServerError)
		return
	}
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req service.InventoryAdjustRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := h.svc.Adjust(r.Context(), h.pool, au.UserID, req); err != nil {
		status := http.StatusBadRequest
		switch err {
		case service.ErrValidation:
			status = http.StatusUnprocessableEntity
		case service.ErrInsufficientStock:
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
