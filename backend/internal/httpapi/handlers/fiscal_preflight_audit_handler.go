package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	fiscalapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Technical actor identities are separate from humans. A lease token is a
// fencing capability and must NEVER be included in the HTTP response.
type FiscalPreflightAuditItem struct {
	ID               int64     `json:"id"`
	WorkerIdentityID string    `json:"worker_identity_id"`
	EventKind        string    `json:"event_kind"`
	Reclaimed        bool      `json:"reclaimed"`
	ResultCode       *string   `json:"result_code,omitempty"`
	RecordedAt       time.Time `json:"recorded_at"`
}

// GetFiscalPreparation exposes the latest immutable local diagnostic and
// verifies whether its data fingerprint is still current. There is NO
// "ready for issuance" status. Cross-company sales are not discoverable.
func (h *SaleFiscalHandler) GetFiscalPreparation(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	saleID := strings.TrimSpace(chi.URLParam(r, "saleID"))
	if _, err := uuid.Parse(saleID); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "venda invalida", nil)
		return
	}
	var exists bool
	err := h.db.QueryRow(r.Context(), `
  SELECT EXISTS(SELECT 1 FROM sale_fiscal_intents WHERE tenant_id=$1 AND sale_id=$2)
 `, au.TenantID, saleID).Scan(&exists)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "falha na consulta do plano fiscal", nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	result, err := fiscalapp.GetLatestFiscalPreparation(r.Context(), h.db, au.TenantID, saleID)
	if errors.Is(err, pgx.ErrNoRows) {
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, map[string]any{
			"sale_id": saleID, "status": "not_prepared", "source_matches": false,
		})
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "falha na consulta do plano fiscal", nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"sale_id": saleID,
		"id":      result.ID, "status": result.Status, "result_code": result.ResultCode,
		"prepared_at": result.PreparedAt, "source_matches": result.SourceMatches,
	})
}

func (h *SaleFiscalHandler) ListFiscalPreflightAudit(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	saleID := strings.TrimSpace(chi.URLParam(r, "saleID"))
	if _, err := uuid.Parse(saleID); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "venda invalida", nil)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "limite de auditoria invalido", nil)
			return
		}
		limit = n
	}
	// First verify the resource exists in the authenticated tenant. No
	// cross-tenant sale or event information may be exposed to callers.
	var exists bool
	err := h.db.QueryRow(r.Context(), `
   SELECT EXISTS(SELECT 1 FROM sale_fiscal_intents
                 WHERE tenant_id=$1 AND sale_id=$2)
 `, au.TenantID, saleID).Scan(&exists)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "falha ao consultar obrigacao fiscal", nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	rows, err := h.db.Query(r.Context(), `
  SELECT e.id,e.worker_identity_id::text,e.event_kind,
    e.reclaimed_from_token IS NOT NULL,e.result_code,e.recorded_at
  FROM fiscal_preflight_audit_events e
  JOIN fiscal_worker_identities w ON w.id=e.worker_identity_id
    AND w.kind='local_preflight'
  WHERE e.tenant_id=$1 AND e.sale_id=$2
  ORDER BY e.id DESC LIMIT $3
 `, au.TenantID, saleID, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "falha na consulta de auditoria fiscal", nil)
		return
	}
	defer rows.Close()
	items := make([]FiscalPreflightAuditItem, 0)
	for rows.Next() {
		var item FiscalPreflightAuditItem
		if err := rows.Scan(&item.ID, &item.WorkerIdentityID, &item.EventKind,
			&item.Reclaimed, &item.ResultCode, &item.RecordedAt); err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", "falha ao ler auditoria fiscal", nil)
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "falha ao ler auditoria fiscal", nil)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"sale_id": saleID,
		"items":   items,
	})
}
