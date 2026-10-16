package handlers

import (
	"net/http"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
)

// FiscalDailyClose is a read-only operational reconciliation, NOT a fiscal
// closure, SEFAZ authorization, transmission or substitute for an accountant.
type FiscalDailyClose struct {
	Date           string `json:"date"`
	Total          int64  `json:"total"`
	NFCe           int64  `json:"nfce"`
	NFe            int64  `json:"nfe"`
	AuthorizedNFCe int64  `json:"authorized_nfce"`
	Unresolved     int64  `json:"unresolved"`
	ManualReview   int64  `json:"manual_review"`
	CancelledSales int64  `json:"cancelled_sales"`
	IntegrityAlert int64  `json:"integrity_alert"`
}

func (h *SaleFiscalHandler) DailyClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "não autenticado", nil)
		return
	}
	day := r.URL.Query().Get("date")
	if _, err := time.Parse("2006-01-02", day); err != nil || len(r.URL.Query()) != 1 || len(r.URL.Query()["date"]) != 1 {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "informe a data no formato AAAA-MM-DD", nil)
		return
	}

	// The SAME authorization predicate used by the obligation listing is
	// evaluated on the entire tenant/day, never on a limited result page.
	// Model 55 is deliberately not counted as confirmed authorization: the
	// NFe issuer is not yet production-ready.
	sql := `
WITH fiscal_day AS (
	SELECT f.document_kind, f.legacy_review, s.status AS sale_status,
	  COALESCE((` + obligationAuthorization + `), false) AND f.document_kind='nfce' AS authorized_nfce,
	  i.status AS invoice_status,
	  j.state AS dispatch_state, a.state AS automatic_state, c.state AS consultation_state,
	  (i.status='authorized' AND f.document_kind='nfce'
	    AND NOT COALESCE((` + obligationAuthorization + `),false)) AS integrity_alert
	` + obligationInvoiceJoin + `
	WHERE f.tenant_id=$1
	  AND f.created_at >= ($2::date::timestamp AT TIME ZONE 'America/Sao_Paulo')
	  AND f.created_at < (($2::date + 1)::timestamp AT TIME ZONE 'America/Sao_Paulo')
)
SELECT count(*),
  count(*) FILTER (WHERE document_kind='nfce'),
  count(*) FILTER (WHERE document_kind='nfe'),
  count(*) FILTER (WHERE authorized_nfce),
  count(*) FILTER (WHERE NOT authorized_nfce),
  count(*) FILTER (WHERE NOT authorized_nfce AND
    (legacy_review OR sale_status='cancelled'
      OR COALESCE(invoice_status,'') IN ('rejected','cancelled')
      OR COALESCE(dispatch_state,'')='manual_review'
      OR COALESCE(automatic_state,'')='manual_review'
      OR COALESCE(consultation_state,'')='manual_review'
      OR COALESCE(integrity_alert,false))),
  count(*) FILTER (WHERE sale_status='cancelled'),
  count(*) FILTER (WHERE COALESCE(integrity_alert,false))
FROM fiscal_day
`
	out := FiscalDailyClose{Date: day}
	if err := h.db.QueryRow(r.Context(), sql, au.TenantID, day).Scan(
		&out.Total, &out.NFCe, &out.NFe, &out.AuthorizedNFCe,
		&out.Unresolved, &out.ManualReview, &out.CancelledSales, &out.IntegrityAlert,
	); err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "não foi possível conferir o dia fiscal", nil)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
