package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/google/uuid"
)

type AuditHandler struct {
	svc    *audit.Service
	logger *slog.Logger
}

func NewAuditHandler(svc *audit.Service, logger *slog.Logger) *AuditHandler {
	return &AuditHandler{svc: svc, logger: logger}
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	filter := audit.ListFilter{
		Action:       strings.TrimSpace(q.Get("action")),
		ResourceType: strings.TrimSpace(q.Get("resource_type")),
		ActorUserID:  strings.TrimSpace(q.Get("actor_user_id")),
		Outcome:      strings.TrimSpace(q.Get("outcome")),
		From:         strings.TrimSpace(q.Get("from")),
		To:           strings.TrimSpace(q.Get("to")),
		Limit:        limit,
		Offset:       offset,
	}
	if err := validateAuditFilter(filter); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	items, err := h.svc.List(r.Context(), au.TenantID, filter)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar auditoria", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func validateAuditFilter(filter audit.ListFilter) error {
	if filter.ActorUserID != "" {
		if _, err := uuid.Parse(filter.ActorUserID); err != nil {
			return ErrValidation
		}
	}
	if filter.Outcome != "" {
		switch filter.Outcome {
		case "success", "failure", "unknown":
		default:
			return ErrValidation
		}
	}
	var fromTime, toTime time.Time
	var err error
	if filter.From != "" {
		fromTime, err = time.Parse(time.RFC3339, filter.From)
		if err != nil {
			return ErrValidation
		}
	}
	if filter.To != "" {
		toTime, err = time.Parse(time.RFC3339, filter.To)
		if err != nil {
			return ErrValidation
		}
	}
	if !fromTime.IsZero() && !toTime.IsZero() && fromTime.After(toTime) {
		return ErrValidation
	}
	return nil
}
