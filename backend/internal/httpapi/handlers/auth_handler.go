package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/redis/go-redis/v9"
	"log/slog"
)

type AuthHandler struct {
	cfg    config.Config
	auth   *authapp.AuthService
	audit  *audit.Service
	rdb    *redis.Client
	logger *slog.Logger
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	Token string `json:"token"`
}

type switchTenantRequest struct {
	TenantID string `json:"tenant_id"`
}

func NewAuthHandler(cfg config.Config, auth *authapp.AuthService, auditSvc *audit.Service, rdb *redis.Client, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{cfg: cfg, auth: auth, audit: auditSvc, rdb: rdb, logger: logger}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	var req loginRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	allowed, limitErr := h.allowLoginIdentifier(r, req.Email)
	if limitErr != nil {
		writeError(w, r, http.StatusServiceUnavailable, "service_unavailable", "rate limit backend unavailable", nil)
		return
	}
	if !allowed {
		writeError(w, r, http.StatusTooManyRequests, "rate_limit", "rate limit exceeded", nil)
		return
	}
	resp, user, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		code := "authentication_error"
		message := "credenciais invalidas"
		reason := "invalid_credentials_or_inactive"
		switch {
		case errors.Is(err, common.ErrInvalidCredentials), errors.Is(err, common.ErrInactiveUser):
			// Keep account existence/active state indistinguishable.
		case errors.Is(err, common.ErrForbidden):
			status = http.StatusForbidden
			code = "authorization_error"
			message = "usuario sem acesso a uma loja ativa"
			reason = "no_active_tenant"
		default:
			status = http.StatusServiceUnavailable
			code = "service_unavailable"
			message = "servico de autenticacao indisponivel"
			reason = "auth_backend_unavailable"
		}
		requestID, ip, userAgent := audit.RequestContext(r)
		h.audit.Record(r.Context(), audit.Event{
			Action:       "auth.login",
			ResourceType: "user",
			Outcome:      "failure",
			Metadata:     map[string]any{"reason": reason},
			RequestID:    requestID,
			IP:           ip,
			UserAgent:    userAgent,
		})
		writeError(w, r, status, code, message, nil)
		return
	}
	setRefreshCookie(w, h.cfg, resp.RefreshToken, h.cfg.RefreshTokenTTL)
	requestID, ip, userAgent := audit.RequestContext(r)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     user.TenantID,
		ActorUserID:  user.ID,
		Action:       "auth.login",
		ResourceType: "user",
		ResourceID:   user.ID,
		Outcome:      "success",
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": publicTokenResponse(resp), "user": user})
}

func (h *AuthHandler) allowLoginIdentifier(r *http.Request, email string) (bool, error) {
	identifierHash := middleware.HashRateLimitIdentifier(email)
	ip := middleware.RateLimitByIP(r)
	idAllowed, err := middleware.AllowRateLimit(r.Context(), h.rdb, "auth_login_identifier", identifierHash, h.cfg.RateLimitLoginID, time.Minute, h.cfg.IsProdLike())
	if err != nil {
		return false, err
	}
	combinedAllowed, err := middleware.AllowRateLimit(r.Context(), h.rdb, "auth_login_ip_identifier", ip+":"+identifierHash, h.cfg.RateLimitLoginIPID, time.Minute, h.cfg.IsProdLike())
	if err != nil {
		return false, err
	}
	return idAllowed && combinedAllowed, nil
}

func (h *AuthHandler) Tenants(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	items, err := h.auth.ListUserTenants(r.Context(), au.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel listar as lojas", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "current_tenant_id": au.TenantID})
}

func (h *AuthHandler) SwitchTenant(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}

	var req switchTenantRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	resp, user, err := h.auth.SwitchTenant(
		r.Context(),
		au.UserID,
		au.TenantID,
		req.TenantID,
		refreshTokenFromCookie(r),
	)
	if err != nil {
		switch err {
		case common.ErrValidation:
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "tenant invalido", nil)
		case common.ErrForbidden:
			writeError(w, r, http.StatusForbidden, "authorization_error", "usuario sem acesso a esta loja", nil)
		case common.ErrConflict:
			writeError(w, r, http.StatusConflict, "session_conflict", "sessao alterada em outra aba", nil)
		case common.ErrInactiveUser:
			writeError(w, r, http.StatusForbidden, "authorization_error", "usuario inativo", nil)
		case common.ErrInvalidCredentials:
			clearRefreshCookie(w, h.cfg)
			writeError(w, r, http.StatusUnauthorized, "authentication_error", "sessao invalida", nil)
		default:
			writeError(w, r, http.StatusServiceUnavailable, "service_unavailable", "nao foi possivel trocar de loja", nil)
		}
		return
	}

	setRefreshCookie(w, h.cfg, resp.RefreshToken, h.cfg.RefreshTokenTTL)
	requestID, ip, userAgent := audit.RequestContext(r)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     user.TenantID,
		ActorUserID:  user.ID,
		Action:       "auth.tenant_switch",
		ResourceType: "tenant",
		ResourceID:   user.TenantID,
		Outcome:      "success",
		Metadata:     map[string]any{"from_tenant_id": au.TenantID, "to_tenant_id": user.TenantID},
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": publicTokenResponse(resp), "user": user})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	token := refreshTokenFromCookie(r)
	resp, userID, tenantID, err := h.auth.RefreshWithSubject(r.Context(), token)
	if err != nil {
		clearRefreshCookie(w, h.cfg)
		requestID, ip, userAgent := audit.RequestContext(r)
		h.audit.Record(r.Context(), audit.Event{
			Action:       "auth.refresh",
			ResourceType: "refresh_token",
			Outcome:      "failure",
			Metadata:     map[string]any{"reason": "invalid_or_expired"},
			RequestID:    requestID,
			IP:           ip,
			UserAgent:    userAgent,
		})
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "token invalido", nil)
		return
	}
	setRefreshCookie(w, h.cfg, resp.RefreshToken, h.cfg.RefreshTokenTTL)
	requestID, ip, userAgent := audit.RequestContext(r)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     tenantID,
		ActorUserID:  userID,
		Action:       "auth.refresh",
		ResourceType: "refresh_token",
		Outcome:      "success",
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
	writeJSON(w, http.StatusOK, publicTokenResponse(resp))
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	token := refreshTokenFromCookie(r)
	userID, tenantID, _ := h.auth.IdentifyRefreshToken(token)
	requestID, ip, userAgent := audit.RequestContext(r)

	if err := h.auth.Logout(r.Context(), token); err != nil {
		h.audit.Record(r.Context(), audit.Event{
			TenantID:     tenantID,
			ActorUserID:  userID,
			Action:       "auth.logout",
			ResourceType: "refresh_token",
			Outcome:      "failure",
			Metadata:     map[string]any{"reason": "revocation_unavailable"},
			RequestID:    requestID,
			IP:           ip,
			UserAgent:    userAgent,
		})
		writeError(w, r, http.StatusServiceUnavailable, "service_unavailable", "nao foi possivel revogar a sessao", nil)
		return
	}

	clearRefreshCookie(w, h.cfg)
	h.audit.Record(r.Context(), audit.Event{
		TenantID:     tenantID,
		ActorUserID:  userID,
		Action:       "auth.logout",
		ResourceType: "refresh_token",
		Outcome:      "success",
		RequestID:    requestID,
		IP:           ip,
		UserAgent:    userAgent,
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	info, err := h.auth.GetUserInfo(r.Context(), au.UserID, au.TenantID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "usuario nao encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func publicTokenResponse(resp authapp.TokenResponse) map[string]any {
	return map[string]any{
		"access_token": resp.AccessToken,
		"token_type":   resp.TokenType,
		"expires_in":   resp.ExpiresIn,
	}
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func setRefreshCookie(w http.ResponseWriter, cfg config.Config, token string, ttl time.Duration) {
	if token == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-refresh_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.IsProdLike(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, cfg config.Config) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.IsProdLike(),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func refreshTokenFromCookie(r *http.Request) string {
	c, err := r.Cookie("__Host-refresh_token")
	if err != nil {
		return ""
	}
	return c.Value
}
