package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/jackc/pgx/v5"
)

// Fiscal obligations are durable records, not issued documents. A complete
// signed/authorized SEFAZ response is still required to consider one fulfilled.
type FiscalObligation struct {
	SaleID       string  `json:"sale_id"`
	CreatedAt    string  `json:"created_at"`
	DocumentKind string  `json:"document_kind"`
	SaleStatus   string  `json:"sale_status"`
	Status       string  `json:"status"`
	Authorized   bool    `json:"authorized"`
	LegacyReview bool    `json:"legacy_review"`
	InvoiceID    *string `json:"invoice_id,omitempty"`
}

type FiscalObligationsPage struct {
	Items []FiscalObligation `json:"items"`
	Total int                `json:"total"`
	Limit int                `json:"limit"`
	Offset int               `json:"offset"`
}

// Do not use issued dates, counters, browser flags or a simple invoice row
// to infer that the note is authorized. The predicate is scoped per tenant,
// document model and sale and is shared by the count and page queries.
const obligationInvoiceJoin = `
FROM sale_fiscal_intents f
JOIN sales s ON s.tenant_id=f.tenant_id AND s.id=f.sale_id
LEFT JOIN LATERAL (
	SELECT id, status, model, authorization_protocol, authorized_at, access_key
	FROM invoices
	WHERE tenant_id=f.tenant_id AND sale_id=f.sale_id
	ORDER BY created_at DESC, id DESC LIMIT 1
) i ON true
`

const obligationAuthorization = `
s.status='finalized'
AND i.status='authorized'
AND (
	(f.document_kind='nfce' AND i.model=65) OR
	(f.document_kind='nfe' AND i.model=55)
)
AND length(btrim(COALESCE(i.authorization_protocol,'')))>0
AND i.authorized_at IS NOT NULL
AND length(btrim(COALESCE(i.access_key,'')))>0
`

func (h *SaleFiscalHandler) QueryObligations(ctx context.Context, tenant string, limit, offset int, unresolvedOnly bool) (FiscalObligationsPage, error) {
	if limit < 1 || limit > 100 { limit = 50 }
	if offset < 0 || offset > 100000 { offset = 0 }
	out := FiscalObligationsPage{
		Items: make([]FiscalObligation, 0),
		Limit: limit, Offset: offset,
	}
	where := ` WHERE f.tenant_id=$1 AND (NOT $2::boolean OR NOT COALESCE((` + obligationAuthorization + `), false)) `
	if err := h.db.QueryRow(ctx, `SELECT count(*) `+obligationInvoiceJoin+where, tenant, unresolvedOnly).Scan(&out.Total); err != nil {
		return FiscalObligationsPage{}, err
	}
	rows, err := h.db.Query(ctx, `
		SELECT f.sale_id::text, f.created_at::text, f.document_kind,
			f.legacy_review, s.status,
			i.id::text, i.status,
			COALESCE((`+obligationAuthorization+`), false) AS authorized
		`+obligationInvoiceJoin+where+`
		ORDER BY f.created_at DESC, f.sale_id DESC LIMIT $3 OFFSET $4
	`, tenant, unresolvedOnly, limit, offset)
	if err != nil { return FiscalObligationsPage{}, err }
	defer rows.Close()
	for rows.Next() {
		var item FiscalObligation
		var invStatus *string
		var authorized *bool
		if err := rows.Scan(&item.SaleID, &item.CreatedAt, &item.DocumentKind,
			&item.LegacyReview, &item.SaleStatus,
			&item.InvoiceID, &invStatus, &authorized); err != nil {
			return FiscalObligationsPage{}, err
		}
		item.Authorized=authorized!=nil && *authorized
		switch {
		case item.Authorized:
			item.Status="authorized"
		case item.SaleStatus=="cancelled":
			item.Status="sale_cancelled_review"
		case invStatus!=nil:
			item.Status=*invStatus
		case item.LegacyReview:
			item.Status="legacy_review"
		default:
			item.Status="pending"
		}
		out.Items=append(out.Items,item)
	}
	if err := rows.Err(); err != nil { return FiscalObligationsPage{}, err }
	return out,nil
}

// ListObligations is a read-only manager view; never reserves invoice numbers
// or attempts fiscal transmission. All queries filter on authenticated tenant.
func (h *SaleFiscalHandler) ListObligations(w http.ResponseWriter, r *http.Request) {
	au,ok:=middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w,r,http.StatusUnauthorized,"authentication_error","nao autenticado",nil)
		return
	}
	limit,err:=strconv.Atoi(r.URL.Query().Get("limit"))
	if err!=nil { limit=50 }
	offset:=0
	if raw:=r.URL.Query().Get("offset");raw!="" {
		var parseErr error
		offset,parseErr=strconv.Atoi(raw)
		if parseErr!=nil || offset<0 || offset>100000 {
			writeError(w,r,http.StatusUnprocessableEntity,"validation_error","paginacao fiscal invalida",nil)
			return
		}
	}
	onlyUnresolved:=true
	if raw:=strings.TrimSpace(r.URL.Query().Get("unresolved"));raw!="" {
		onlyUnresolved,err=strconv.ParseBool(raw)
		if err!=nil {
			writeError(w,r,http.StatusUnprocessableEntity,"validation_error","filtro fiscal invalido",nil)
			return
		}
	}
	out,err:=h.QueryObligations(r.Context(),au.TenantID,limit,offset,onlyUnresolved)
	if err!=nil {
		writeError(w,r,http.StatusInternalServerError,"internal_error","falha na consulta das pendencias fiscais",nil)
		return
	}
	writeJSON(w,http.StatusOK,out)
}

