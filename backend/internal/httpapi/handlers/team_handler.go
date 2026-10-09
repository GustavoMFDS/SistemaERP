package handlers

import (
	"errors"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/team"
	"github.com/go-chi/chi/v5"
)

type TeamHandler struct {
	svc *team.Service
}

func NewTeamHandler(svc *team.Service) *TeamHandler {
	return &TeamHandler{svc: svc}
}

type inviteStaffRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type acceptStaffRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type setStaffRoleRequest struct {
	Role string `json:"role"`
}

type setStaffStatusRequest struct {
	Active *bool `json:"active"`
}

func writeTeamError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "dados da equipe invalidos", nil)
	case errors.Is(err, common.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "authorization_error", "acao nao permitida para esta conta", nil)
	case errors.Is(err, common.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "registro nao encontrado nesta loja", nil)
	case errors.Is(err, common.ErrConflict):
		writeError(w, r, http.StatusConflict, "conflict", "nao foi possivel criar ou atualizar este acesso", nil)
	case errors.Is(err, team.ErrInviteUnavailable):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "convite invalido, expirado ou utilizado", nil)
	default:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel concluir a operacao", nil)
	}
}

func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	members, invitations, err := h.svc.List(r.Context(), au.TenantID)
	if err != nil {
		writeTeamError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members, "invitations": invitations})
}

func (h *TeamHandler) Invite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req inviteStaffRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "dados de convite invalidos", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	invite, secret, err := h.svc.Invite(r.Context(), au.TenantID, au.UserID,
		req.Name, req.Email, req.Role, requestID, ip, agent)
	if err != nil {
		writeTeamError(w, r, err)
		return
	}
	// This secret is shown exactly once, never listed or logged server-side.
	writeJSON(w, http.StatusCreated, map[string]any{"invitation": invite, "token": secret})
}

func (h *TeamHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	if err := h.svc.Revoke(r.Context(), au.TenantID, au.UserID, chi.URLParam(r, "id"), requestID, ip, agent); err != nil {
		writeTeamError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) SetRole(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req setStaffRoleRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "papel invalido", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	if err := h.svc.UpdateMember(r.Context(), au.TenantID, au.UserID,
		chi.URLParam(r, "id"), req.Role, nil, requestID, ip, agent); err != nil {
		writeTeamError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req setStaffStatusRequest
	if err := readJSON(w, r, &req); err != nil || req.Active == nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "informe active true ou false", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	if err := h.svc.UpdateMember(r.Context(), au.TenantID, au.UserID,
		chi.URLParam(r, "id"), "", req.Active, requestID, ip, agent); err != nil {
		writeTeamError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) Accept(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var req acceptStaffRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", "convite invalido", nil)
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	if err := h.svc.Accept(r.Context(), req.Token, req.Password, requestID, ip, agent); err != nil {
		writeTeamError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "registered"})
}
