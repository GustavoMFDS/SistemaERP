package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	privacyapp "github.com/example/sistemaemgo/internal/modules/privacy/application"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type PrivacyHandler struct {
	svc    *privacyapp.Service
	audit  *audit.Service
	logger *slog.Logger
}

func NewPrivacyHandler(svc *privacyapp.Service, auditSvc *audit.Service, logger *slog.Logger) *PrivacyHandler {
	return &PrivacyHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *PrivacyHandler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req privacyapp.CreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, err := h.svc.CreateRequest(r.Context(), au.TenantID, au.UserID, middleware.GetRequestID(r.Context()), req)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *PrivacyHandler) ListRequests(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.svc.ListRequests(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *PrivacyHandler) GetRequest(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	item, err := h.svc.GetRequest(r.Context(), au.TenantID, id)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *PrivacyHandler) UpdateRequestStatus(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req privacyapp.UpdateStatusRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := h.svc.UpdateRequestStatus(r.Context(), au.TenantID, au.UserID, id, req); err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "updated"})
}

func (h *PrivacyHandler) ExportSubject(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	data, err := h.svc.ExportSubjectData(r.Context(), au.TenantID, id)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	h.record(r, au.TenantID, au.UserID, "privacy.subject.export", "data_subject_request", id, "success", nil)
	writeJSON(w, http.StatusOK, map[string]any{"request_id": id, "data": data})
}

func (h *PrivacyHandler) AnonymizeSubject(w http.ResponseWriter, r *http.Request) {
	h.subjectAction(w, r, func(tenantID, actorUserID, requestID string) error {
		return h.svc.AnonymizeSubject(r.Context(), tenantID, actorUserID, requestID)
	})
}

func (h *PrivacyHandler) BlockSubject(w http.ResponseWriter, r *http.Request) {
	h.subjectAction(w, r, func(tenantID, actorUserID, requestID string) error {
		return h.svc.BlockSubject(r.Context(), tenantID, actorUserID, requestID)
	})
}

func (h *PrivacyHandler) RecordConsent(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req privacyapp.ConsentCreateRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	id, err := h.svc.RecordConsent(r.Context(), au.TenantID, au.UserID, middleware.GetRequestID(r.Context()), req)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *PrivacyHandler) ListConsents(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, err := h.svc.ListConsents(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *PrivacyHandler) RevokeConsent(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if err := h.svc.RevokeConsent(r.Context(), au.TenantID, au.UserID, id); err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "revoked"})
}

func (h *PrivacyHandler) subjectAction(w http.ResponseWriter, r *http.Request, fn func(tenantID, actorUserID, requestID string) error) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, valid := requireUUID(w, r, chi.URLParam(r, "id"))
	if !valid {
		return
	}
	if err := fn(au.TenantID, au.UserID, id); err != nil {
		writePrivacyError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "completed"})
}

func (h *PrivacyHandler) record(r *http.Request, tenantID, actorID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	requestID, ip, userAgent := audit.RequestContext(r)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Outcome:      outcome,
		Metadata:     metadata,
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
}

func writePrivacyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "dados invalidos", nil)
	case errors.Is(err, common.ErrConflict):
		writeError(w, r, http.StatusConflict, "conflict", "operacao conflita com o estado atual do recurso", nil)
	case errors.Is(err, common.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		writeError(w, r, http.StatusNotFound, "not_found", "recurso nao encontrado", nil)
	default:
		writeError(w, r, http.StatusBadRequest, "validation_error", "nao foi possivel processar a solicitacao", nil)
	}
}
