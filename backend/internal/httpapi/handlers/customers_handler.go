package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/customers"
	"github.com/go-chi/chi/v5"
)

type CustomersHandler struct {
	svc *customers.Service
}

func NewCustomersHandler(svc *customers.Service) *CustomersHandler {
	return &CustomersHandler{svc: svc}
}

func writeCustomerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "dados ou consulta invalidos", nil)
	case errors.Is(err, common.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "cliente nao encontrado nesta empresa", nil)
	case errors.Is(err, common.ErrConflict):
		writeError(w, r, http.StatusConflict, "conflict", "cadastro em conflito", nil)
	default:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar ou salvar cliente", nil)
	}
}

func (h *CustomersHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	query := r.URL.Query()
	for name, values := range query {
		if name != "q" && name != "limit" && name != "offset" {
			writeCustomerError(w, r, common.ErrValidation)
			return
		}
		if len(values) != 1 {
			writeCustomerError(w, r, common.ErrValidation)
			return
		}
	}
	limit, offset := 20, 0
	if value, exists := query["limit"]; exists {
		number, err := strconv.Atoi(value[0])
		if err != nil || number < 1 || number > 50 {
			writeCustomerError(w, r, common.ErrValidation)
			return
		}
		limit = number
	}
	if value, exists := query["offset"]; exists {
		number, err := strconv.Atoi(value[0])
		if err != nil || number < 0 || number > 5000 {
			writeCustomerError(w, r, common.ErrValidation)
			return
		}
		offset = number
	}
	result, err := h.svc.List(r.Context(), au.TenantID, query.Get("q"), limit, offset)
	if err != nil {
		writeCustomerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CustomersHandler) Create(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var input customers.CustomerInput
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "dados invalidos", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	result, err := h.svc.Create(r.Context(), au.TenantID, au.UserID, input, requestID, ip, agent)
	if err != nil {
		writeCustomerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *CustomersHandler) Update(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var input customers.CustomerInput
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "dados invalidos", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	result, err := h.svc.Update(r.Context(), au.TenantID, au.UserID,
		chi.URLParam(r, "id"), input, requestID, ip, agent)
	if err != nil {
		writeCustomerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
