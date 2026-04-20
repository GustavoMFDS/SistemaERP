package handlers

import (
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/go-chi/chi/v5"
)

type ProductsHandler struct {
	svc    *service.ProductsService
	logger *slog.Logger
	pool   *pgxpool.Pool
}

func NewProductsHandler(svc *service.ProductsService, logger *slog.Logger) *ProductsHandler {
	return &ProductsHandler{svc: svc, logger: logger}
}

func (h *ProductsHandler) BindDB(pool *pgxpool.Pool) {
	h.pool = pool
}

func (h *ProductsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.List(r.Context(), q, limit, offset)
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *ProductsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProductsHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "db not configured", http.StatusInternalServerError)
		return
	}
	var req service.ProductCreateRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	id, err := h.svc.Create(r.Context(), tx, req)
	if err != nil {
		status := http.StatusBadRequest
		if err == service.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *ProductsHandler) Update(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "db not configured", http.StatusInternalServerError)
		return
	}
	id := chi.URLParam(r, "id")
	var req service.ProductUpdateRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if err := h.svc.Update(r.Context(), tx, id, req); err != nil {
		status := http.StatusBadRequest
		if err == service.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		http.Error(w, err.Error(), status)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}
