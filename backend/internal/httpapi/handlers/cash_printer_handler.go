package handlers

import (
	"errors"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GET /cash/sessions/{id}/printer reads the persistent printer paper
// preference from the actual tenant-owned register of an open session.
func (h *SaleFiscalHandler) GetCashPrinter(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(sessionID); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "ID da sessao invalido", nil)
		return
	}
	var registerID string
	var width int
	err := h.db.QueryRow(r.Context(), `
		SELECT cr.id::text,cr.danfe_paper_width_mm
		FROM cash_sessions cs JOIN cash_registers cr
		ON cr.id=cs.cash_register_id AND cr.tenant_id=cs.tenant_id
		WHERE cs.tenant_id=$1 AND cs.id=$2 AND cs.status='open'
	`, au.TenantID, sessionID).Scan(&registerID, &width)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "caixa nao aberto nesta loja", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar preferencia de impressao", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cash_register_id": registerID,
		"paper_width_mm":   width,
	})
}

type cashPrinterUpdate struct {
	PaperWidthMM int `json:"paper_width_mm"`
}

// PUT /cash/sessions/{id}/printer cannot modify any other tenant or closed
// session; it commits the preference with the actor's audit record.
func (h *SaleFiscalHandler) SetCashPrinter(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	sessionID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(sessionID); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "ID da sessao invalido", nil)
		return
	}
	var req cashPrinterUpdate
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if req.PaperWidthMM != 58 && req.PaperWidthMM != 80 {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "largura aceita: 58 ou 80 mm", nil)
		return
	}
	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao gravar preferencia", nil)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var registerID string
	err = tx.QueryRow(ctx, `
		UPDATE cash_registers cr
		SET danfe_paper_width_mm=$3
		FROM cash_sessions cs
		WHERE cs.tenant_id=$1 AND cs.id=$2 AND cs.status='open'
		  AND cs.cash_register_id=cr.id AND cr.tenant_id=$1
		RETURNING cr.id::text
	`, au.TenantID, sessionID, req.PaperWidthMM).Scan(&registerID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "caixa nao aberto nesta loja", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao atualizar impressora", nil)
		return
	}
	if h.audit != nil {
		if err := h.audit.RecordTx(ctx, tx, audit.Event{
			TenantID: au.TenantID, ActorUserID: au.UserID,
			Action: "cash.printer.paper_width.updated", ResourceType: "cash_register",
			ResourceID: registerID, Outcome: "success",
			Metadata: map[string]any{"paper_width_mm": req.PaperWidthMM},
		}); err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", "erro de auditoria ao configurar impressora", nil)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao salvar preferencia de bobina", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cash_register_id": registerID,
		"paper_width_mm":   req.PaperWidthMM,
	})
}
