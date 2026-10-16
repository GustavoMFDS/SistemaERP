package handlers

import (
	"net/http"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
)

type SalesByHour struct {
	Hour  int     `json:"hour"`
	Count int64   `json:"count"`
	Total float64 `json:"total"`
}

type ProductMargin struct {
	SKU             string  `json:"sku"`
	Name            string  `json:"name"`
	Quantity        float64 `json:"quantity"`
	RevenueItems    float64 `json:"revenue_items"`
	EstimatedMargin float64 `json:"estimated_margin"`
}

type UnsoldProduct struct {
	SKU  string `json:"sku"`
	Name string `json:"name"`
}

type OperationalReport struct {
	From              string          `json:"from"`
	To                string          `json:"to"`
	Hourly            []SalesByHour   `json:"hourly"`
	Margins           []ProductMargin `json:"margins"`
	Unsold            []UnsoldProduct `json:"unsold"`
	UnsoldTotal       int64           `json:"unsold_total"`
	CashDiscrepancies int64           `json:"cash_discrepancies"`
	OverdueAccounts   int64           `json:"overdue_accounts"`
}

// OperationalReports never takes tenant identifiers from the caller and never
// books payments. Margins are estimates BEFORE sale-level discounts, returns,
// taxes and operating costs, NOT accounting profit.
func (h *FinanceAccountsHandler) OperationalReports(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		accountFailure(w, r, http.StatusUnauthorized, "Sessão inválida")
		return
	}
	q := r.URL.Query()
	if len(q) != 2 || len(q["from"]) != 1 || len(q["to"]) != 1 {
		accountFailure(w, r, http.StatusUnprocessableEntity, "Informe um período válido")
		return
	}
	from, e1 := time.Parse("2006-01-02", q.Get("from"))
	to, e2 := time.Parse("2006-01-02", q.Get("to"))
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 92*24*time.Hour {
		accountFailure(w, r, http.StatusUnprocessableEntity, "Período inválido ou superior a 93 dias")
		return
	}
	out := OperationalReport{
		From: q.Get("from"), To: q.Get("to"),
		Hourly: make([]SalesByHour, 0, 24), Margins: make([]ProductMargin, 0),
		Unsold: make([]UnsoldProduct, 0),
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT EXTRACT(HOUR FROM s.finalized_at AT TIME ZONE 'America/Sao_Paulo')::integer,
		  COUNT(*), COALESCE(SUM(s.total),0)::float8
		FROM sales s
		WHERE s.tenant_id=$1 AND s.status='finalized'
		  AND (s.finalized_at AT TIME ZONE 'America/Sao_Paulo')::date BETWEEN $2::date AND $3::date
		GROUP BY 1 ORDER BY 1
	`, au.TenantID, out.From, out.To)
	if err != nil {
		accountFailure(w, r, 500, "Falha na consulta de vendas por horário")
		return
	}
	for rows.Next() {
		var item SalesByHour
		if err = rows.Scan(&item.Hour, &item.Count, &item.Total); err != nil { break }
		out.Hourly = append(out.Hourly, item)
	}
	if err == nil { err = rows.Err() }
	rows.Close()
	if err != nil {
		accountFailure(w, r, 500, "Falha ao ler vendas por horário")
		return
	}
	rows, err = h.pool.Query(r.Context(), `
		SELECT p.sku, p.name, COALESCE(SUM(si.qty),0)::float8,
		  COALESCE(SUM(si.subtotal),0)::float8,
		  COALESCE(SUM(si.subtotal - si.qty*COALESCE(si.cost_unit,0)),0)::float8
		FROM sale_items si
		JOIN sales s ON s.tenant_id=si.tenant_id AND s.id=si.sale_id
		JOIN products p ON p.tenant_id=si.tenant_id AND p.id=si.product_id
		WHERE si.tenant_id=$1 AND s.status='finalized'
		  AND (s.finalized_at AT TIME ZONE 'America/Sao_Paulo')::date BETWEEN $2::date AND $3::date
		GROUP BY p.id,p.sku,p.name ORDER BY SUM(si.subtotal) DESC, p.sku ASC LIMIT 10
	`, au.TenantID, out.From, out.To)
	if err != nil {
		accountFailure(w, r, 500, "Falha no resumo por produto")
		return
	}
	for rows.Next() {
		var item ProductMargin
		if err = rows.Scan(&item.SKU, &item.Name, &item.Quantity, &item.RevenueItems, &item.EstimatedMargin); err != nil { break }
		out.Margins = append(out.Margins, item)
	}
	if err == nil { err = rows.Err() }
	rows.Close()
	if err != nil {
		accountFailure(w, r, 500, "Falha ao ler resumo de produtos")
		return
	}
	const noSale = `
		FROM products p
		WHERE p.tenant_id=$1 AND p.active
		  AND NOT EXISTS (
		    SELECT 1 FROM sale_items si
		    JOIN sales s ON s.tenant_id=si.tenant_id AND s.id=si.sale_id
		    WHERE si.tenant_id=p.tenant_id AND si.product_id=p.id
		      AND s.status='finalized'
		      AND (s.finalized_at AT TIME ZONE 'America/Sao_Paulo')::date BETWEEN $2::date AND $3::date
		  )`
	if err = h.pool.QueryRow(r.Context(), "SELECT COUNT(*)"+noSale,
		au.TenantID, out.From, out.To).Scan(&out.UnsoldTotal); err != nil {
		accountFailure(w, r, 500, "Falha ao contar produtos sem vendas")
		return
	}
	rows, err = h.pool.Query(r.Context(), "SELECT p.sku, p.name"+noSale+" ORDER BY p.name,p.sku LIMIT 10",
		au.TenantID, out.From, out.To)
	if err != nil {
		accountFailure(w, r, 500, "Falha ao listar produtos sem vendas")
		return
	}
	for rows.Next() {
		var item UnsoldProduct
		if err = rows.Scan(&item.SKU, &item.Name); err != nil { break }
		out.Unsold = append(out.Unsold, item)
	}
	if err == nil { err = rows.Err() }
	rows.Close()
	if err != nil {
		accountFailure(w, r, 500, "Falha ao ler produtos sem vendas")
		return
	}
	err = h.pool.QueryRow(r.Context(), `
		SELECT count(*) FROM cash_sessions
		WHERE tenant_id=$1 AND status='closed'
		  AND closed_at IS NOT NULL AND COALESCE(closing_difference,0)<>0
		  AND (closed_at AT TIME ZONE 'America/Sao_Paulo')::date BETWEEN $2::date AND $3::date
	`, au.TenantID, out.From, out.To).Scan(&out.CashDiscrepancies)
	if err != nil {
		accountFailure(w, r, 500, "Falha na contagem de diferenças de caixa")
		return
	}
	err = h.pool.QueryRow(r.Context(), `
		SELECT
		  (SELECT count(*) FROM accounts_payable
		   WHERE tenant_id=$1 AND status='open' AND due_date < (now() AT TIME ZONE 'America/Sao_Paulo')::date)
		  +
		  (SELECT count(*) FROM accounts_receivable
		   WHERE tenant_id=$1 AND status='open' AND due_date < (now() AT TIME ZONE 'America/Sao_Paulo')::date)
	`, au.TenantID).Scan(&out.OverdueAccounts)
	if err != nil {
		accountFailure(w, r, 500, "Falha na contagem de contas vencidas")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
