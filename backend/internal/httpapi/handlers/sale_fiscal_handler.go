package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SaleFiscalHandler exposes a cashier-safe fiscal status and strictly
// authorized DANFE access. It does not reserve numbers, issue notes or
// bypass the production SEFAZ gates.
type SaleFiscalHandler struct {
	db     *pgxpool.Pool
	fiscal *fiscapp.FiscalService
	audit  *audit.Service
}

func NewSaleFiscalHandler(pool *pgxpool.Pool, fiscal *fiscapp.FiscalService, auditSvc *audit.Service) *SaleFiscalHandler {
	return &SaleFiscalHandler{db: pool, fiscal: fiscal, audit: auditSvc}
}

type saleFiscalStatus struct {
	SaleID       string  `json:"sale_id"`
	Required     bool    `json:"required"`
	DocumentKind string  `json:"document_kind"`
	Status       string  `json:"status"`
	Authorized   bool    `json:"authorized"`
	Printable    bool    `json:"printable"`
	InvoiceID    *string `json:"invoice_id,omitempty"`
	AccessKey    *string `json:"access_key,omitempty"`
	LegacyReview bool    `json:"legacy_review"`
}

// Status is derived from actual invoice authorization state, not an optimistic
// browser flag. A pending fiscal obligation remains pending whether the
// customer accepts printed paper or not.
func (h *SaleFiscalHandler) loadStatus(r *http.Request, tenantID, saleID string) (saleFiscalStatus, error) {
	out := saleFiscalStatus{SaleID: saleID, Required: true, DocumentKind: "nfce", Status: "pending"}
	var saleStatus string
	var kind *string
	var legacy *bool
	var invoiceID *string
	var invoiceStatus *string
	var invoiceModel *int16
	var protocol *string
	var accessKey *string
	var authorizedAt *string
	err := h.db.QueryRow(r.Context(), `
		SELECT s.status,
			f.document_kind, f.legacy_review,
			i.id::text, i.status, i.model,
			i.authorization_protocol, i.authorized_at::text, i.access_key
		FROM sales s
		LEFT JOIN sale_fiscal_intents f ON f.tenant_id=s.tenant_id AND f.sale_id=s.id
		LEFT JOIN LATERAL (
			SELECT id, status, model, authorization_protocol, authorized_at, access_key
			FROM invoices
			WHERE tenant_id=s.tenant_id AND sale_id=s.id
			ORDER BY created_at DESC, id DESC LIMIT 1
		) i ON true
		WHERE s.tenant_id=$1 AND s.id=$2
	`, tenantID, saleID).Scan(&saleStatus, &kind, &legacy, &invoiceID, &invoiceStatus, &invoiceModel, &protocol, &authorizedAt, &accessKey)
	if err != nil {
		return out, err
	}
	if kind != nil {
		out.DocumentKind = *kind
	}
	if legacy != nil {
		out.LegacyReview = *legacy
	} else {
		out.LegacyReview = true
	}
	if saleStatus == "cancelled" {
		out.Status = "sale_cancelled"
	}
	if invoiceStatus != nil {
		out.Status = *invoiceStatus
		if invoiceID != nil {
			out.InvoiceID = invoiceID
		}
	} else if out.LegacyReview {
		out.Status = "legacy_review"
	}
	if saleStatus == "finalized" && invoiceStatus != nil && *invoiceStatus == "authorized" &&
		invoiceModel != nil && *invoiceModel == 65 &&
		protocol != nil && strings.TrimSpace(*protocol) != "" &&
		authorizedAt != nil && *authorizedAt != "" &&
		accessKey != nil && strings.TrimSpace(*accessKey) != "" && out.DocumentKind == "nfce" {
		out.Authorized = true
		out.Printable = true
		out.AccessKey = accessKey
	}
	// Cancelled sales or voided invoices are never offered for ordinary print.
	return out, nil
}

func (h *SaleFiscalHandler) Status(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "ID de venda invalido", nil)
		return
	}
	status, err := h.loadStatus(r, au.TenantID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar situacao fiscal", nil)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *SaleFiscalHandler) PrintDANFE(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "ID de venda invalido", nil)
		return
	}
	state, err := h.loadStatus(r, au.TenantID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar situacao fiscal", nil)
		return
	}
	if !state.Printable || state.InvoiceID == nil || h.fiscal == nil {
		writeError(w, r, http.StatusConflict, "conflict", "DANFE indisponivel: NFC-e ainda nao autorizada", nil)
		return
	}
	name, data, err := h.fiscal.RenderNFCeDANFE(r.Context(), au.TenantID, *state.InvoiceID)
	if err != nil {
		writeError(w, r, http.StatusConflict, "conflict", "NFC-e nao esta pronta para impressao", nil)
		return
	}
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfce.danfe.download",
		"invoice", *state.InvoiceID, "success", nil)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
