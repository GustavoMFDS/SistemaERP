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
	var processedXMLPresent bool
	err := h.db.QueryRow(r.Context(), `
		SELECT s.status,
			f.document_kind, f.legacy_review,
			i.id::text, i.status, i.model,
			i.authorization_protocol, i.authorized_at::text, i.access_key,
			EXISTS (
			  SELECT 1 FROM invoice_authorized_xml_files x
			  WHERE x.tenant_id=s.tenant_id AND x.invoice_id=i.id
			    AND x.access_key=i.access_key
			)
		FROM sales s
		LEFT JOIN sale_fiscal_intents f ON f.tenant_id=s.tenant_id AND f.sale_id=s.id
		LEFT JOIN LATERAL (
			SELECT id, status, model, authorization_protocol, authorized_at, access_key
			FROM invoices
			WHERE tenant_id=s.tenant_id AND sale_id=s.id
			ORDER BY created_at DESC, id DESC LIMIT 1
		) i ON true
		WHERE s.tenant_id=$1 AND s.id=$2
	`, tenantID, saleID).Scan(&saleStatus, &kind, &legacy, &invoiceID, &invoiceStatus, &invoiceModel, &protocol, &authorizedAt, &accessKey, &processedXMLPresent)
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
		authorizedAt != nil && *authorizedAt != "" && processedXMLPresent &&
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

// DownloadSaleProcessedXML lets a cashier recover the legally relevant
// nfeProc independently of printing a DANFE. It requires the SAME validated,
// tenant-scoped sale and archived authorization as the print route; the service
// then re-verifies the archived XML's SHA-256 before returning any bytes.
// Pending, cancelled, offline and model-55 documents are not downloadable
// as an "authorized NFC-e" through this cashier route.
func (h *SaleFiscalHandler) DownloadSaleProcessedXML(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	saleID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(saleID); err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "ID de venda invalido", nil)
		return
	}
	state, err := h.loadStatus(r, au.TenantID, saleID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "not_found", "venda nao encontrada", nil)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar situacao fiscal", nil)
		return
	}
	if !state.Authorized || !state.Printable || state.InvoiceID == nil ||
		state.AccessKey == nil || h.fiscal == nil {
		writeError(w, r, http.StatusConflict, "conflict", "XML indisponivel: NFC-e nao autorizada e arquivada", nil)
		return
	}
	_, content, err := h.fiscal.DownloadAuthorizedNFCeProcessedXML(r.Context(), au.TenantID, *state.InvoiceID)
	if err != nil {
		writeError(w, r, http.StatusConflict, "conflict", "XML autorizado indisponivel ou com integridade comprometida", nil)
		return
	}
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfce.nfe_proc.cashier_download",
		"invoice", *state.InvoiceID, "success", nil)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+*state.InvoiceID+"-procNFe.xml\"")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
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
	// Derive the saved paper width from the register that originally made
	// the sale. Never trust an arbitrary paper-width URL parameter.
	var paperWidthMM int
	err = h.db.QueryRow(r.Context(), `
		SELECT cr.danfe_paper_width_mm
		FROM sales s
		JOIN cash_sessions cs ON cs.tenant_id=s.tenant_id
		  AND cs.id=s.cash_session_id
		JOIN cash_registers cr ON cr.tenant_id=cs.tenant_id
		  AND cr.id=cs.cash_register_id
		WHERE s.tenant_id=$1 AND s.id=$2
	`, au.TenantID, id).Scan(&paperWidthMM)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "largura de bobina do caixa indisponivel", nil)
		return
	}
	name, data, err := h.fiscal.RenderNFCeDANFE(r.Context(), au.TenantID, *state.InvoiceID, paperWidthMM)
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
