package handlers

import (
	"errors"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/setup"
	"github.com/go-chi/chi/v5"
)

type SetupHandler struct {
	svc *setup.Service
}

func NewSetupHandler(svc *setup.Service) *SetupHandler {
	return &SetupHandler{svc: svc}
}

type setupReviewRequest struct {
	Reviewed *bool `json:"reviewed"`
}

func (h *SetupHandler) ListReviews(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	items, err := h.svc.List(r.Context(), au.TenantID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar revisoes", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *SetupHandler) SetReview(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	step := chi.URLParam(r, "step")
	if !setup.ValidStep(step) {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "etapa nao pode ser revisada manualmente", nil)
		return
	}
	var req setupReviewRequest
	if err := readJSON(w, r, &req); err != nil || req.Reviewed == nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "informe reviewed como booleano", nil)
		return
	}
	requestID, ip, userAgent := audit.RequestContext(r)
	if err := h.svc.Set(r.Context(), au.TenantID, au.UserID, step, *req.Reviewed, requestID, ip, userAgent); err != nil {
		if errors.Is(err, setup.ErrInvalidStep) {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "etapa invalida", nil)
		} else {
			writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel salvar a revisao", nil)
		}
		return
	}
	h.ListReviews(w, r)
}
