package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/google/uuid"
)

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string, message string, details any) {
	writeJSON(w, status, APIError{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: middleware.GetRequestID(r.Context()),
	})
}

const maxJSONBodyBytes = 1 << 20

func normalizeUUID(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if _, err := uuid.Parse(value); err != nil {
		return "", false
	}
	return value, true
}

func validUUID(raw string) bool {
	_, ok := normalizeUUID(raw)
	return ok
}

func requireUUID(w http.ResponseWriter, r *http.Request, raw string) (string, bool) {
	value, ok := normalizeUUID(raw)
	if !ok {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "identificador invalido", nil)
		return "", false
	}
	return value, true
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("%w: invalid JSON near byte %d", ErrValidation, syntaxErr.Offset)
		case errors.As(err, &typeErr):
			return fmt.Errorf("%w: invalid value for field %q", ErrValidation, typeErr.Field)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return fmt.Errorf("%w: invalid JSON", ErrValidation)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return fmt.Errorf("%w: %s", ErrValidation, err.Error())
		case err.Error() == "http: request body too large":
			return fmt.Errorf("%w: request body too large", ErrValidation)
		case errors.Is(err, io.EOF):
			return fmt.Errorf("%w: request body is required", ErrValidation)
		default:
			return fmt.Errorf("%w: invalid JSON", ErrValidation)
		}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request body must contain only one JSON value", ErrValidation)
	}
	return nil
}

var ErrNotFound = errors.New("not found")
var ErrForbidden = errors.New("forbidden")
var ErrUnauthorized = errors.New("unauthorized")
var ErrConflict = errors.New("conflict")
var ErrValidation = errors.New("validation")

func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "authorization_error"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate_limit"
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return "validation_error"
	default:
		return "internal_error"
	}
}

func friendlyErrorMessage(err error) string {
	switch {
	case errors.Is(err, common.ErrValidation):
		return "dados invalidos"
	case errors.Is(err, common.ErrInsufficientStock):
		return "estoque insuficiente"
	case errors.Is(err, common.ErrInsufficientCash):
		return "saldo de caixa insuficiente"
	case errors.Is(err, common.ErrCashSessionClosed):
		return "sessao de caixa fechada"
	case errors.Is(err, common.ErrCashSessionAlreadyOpen):
		return "ja existe uma sessao aberta para este caixa"
	case errors.Is(err, common.ErrPaymentsMismatch):
		return "pagamentos nao conferem com o total"
	case errors.Is(err, common.ErrConflict):
		return "conflito com o estado atual do recurso"
	case errors.Is(err, common.ErrNotFound):
		return "recurso nao encontrado"
	default:
		return "nao foi possivel processar a solicitacao"
	}
}

func recordAudit(auditSvc *audit.Service, r *http.Request, tenantID, actorID, action, resourceType, resourceID, outcome string, metadata map[string]any) {
	requestID, ip, userAgent := audit.RequestContext(r)
	auditSvc.Record(r.Context(), audit.Event{
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
