package handlers

import (
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/service"
	"log/slog"
)

type AuthHandler struct {
	cfg    config.Config
	auth   *service.AuthService
	logger *slog.Logger
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	Token string `json:"token"`
}

func NewAuthHandler(cfg config.Config, auth *service.AuthService, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{cfg: cfg, auth: auth, logger: logger}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	resp, user, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if err == service.ErrInactiveUser {
			status = http.StatusForbidden
		}
		http.Error(w, "invalid credentials", status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": resp, "user": user})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := h.auth.Refresh(r.Context(), req.Token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	info, err := h.auth.GetUserInfo(r.Context(), au.UserID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
