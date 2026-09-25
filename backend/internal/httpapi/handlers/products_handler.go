package handlers

import (
	"net/http"
	"strconv"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"

	"github.com/go-chi/chi/v5"
)

type ProductsHandler struct {
	svc    *invapp.ProductsService
	audit  *audit.Service
	logger *slog.Logger
}

func NewProductsHandler(svc *invapp.ProductsService, auditSvc *audit.Service, logger *slog.Logger) *ProductsHandler {
	return &ProductsHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *ProductsHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	q := r.URL.Query().Get("query")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.List(r.Context(), au.TenantID, q, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar produtos", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *ProductsHandler) Get(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), au.TenantID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "produto nao encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProductsHandler) Create(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req invapp.ProductCreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, err := h.svc.Create(r.Context(), au.TenantID, req)
	if err != nil {
		status := http.StatusBadRequest
		if err == common.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	recordAudit(h.audit, r, au.TenantID, au.UserID, "product.create", "product", id, "success", nil)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *ProductsHandler) Update(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req invapp.ProductUpdateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.Update(r.Context(), au.TenantID, id, req); err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	recordAudit(h.audit, r, au.TenantID, au.UserID, "product.update", "product", id, "success", nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}
