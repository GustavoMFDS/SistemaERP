package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FinanceAccountsHandler controls manual receivables/payables. It does not
// authorize bank movements or infer that Pix/card payments were confirmed.
type FinanceAccountsHandler struct {
	pool  *pgxpool.Pool
	audit *audit.Service
}

func NewFinanceAccountsHandler(pool *pgxpool.Pool, auditSvc *audit.Service) *FinanceAccountsHandler {
	return &FinanceAccountsHandler{pool: pool, audit: auditSvc}
}

type financeAccount struct {
	ID               string         `json:"id"`
	Kind             string         `json:"kind"`
	Description      string         `json:"description"`
	Amount           platform.Money `json:"amount"`
	DueDate          string         `json:"due_date"`
	Status           string         `json:"status"`
	SettledAt        *time.Time     `json:"settled_at"`
	SettlementMethod *string        `json:"settlement_method"`
	PurchaseID       *string        `json:"purchase_id,omitempty"`
}

func accountTable(kind string) (string, string, bool) {
	switch kind {
	case "payable":
		return "accounts_payable", "paid", true
	case "receivable":
		return "accounts_receivable", "received", true
	default:
		return "", "", false
	}
}

func accountFailure(w http.ResponseWriter, r *http.Request, status int, msg string) {
	writeError(w, r, status, errorCodeForStatus(status), msg, nil)
}

func accountKey(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return "", false
	}
	return id.String(), true
}

func (h *FinanceAccountsHandler) List(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		accountFailure(w, r, http.StatusUnauthorized, "Sessão inválida")
		return
	}
	kind := r.URL.Query().Get("kind")
	var where string
	switch kind {
	case "", "all":
		where = ""
	case "payable", "receivable":
		where = "WHERE kind='" + kind + "'"
	default:
		accountFailure(w, r, http.StatusUnprocessableEntity, "Tipo de conta inválido")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id::text, kind, description, amount::text, due_date::text,
		       status, settled_at, settlement_method, purchase_id
		FROM (
		  SELECT id, 'payable' AS kind, description, amount, due_date, status,
		         settled_at, settlement_method, purchase_id::text AS purchase_id
		  FROM accounts_payable WHERE tenant_id=$1
		  UNION ALL
		  SELECT id, 'receivable', description, amount, due_date, status,
		         settled_at, settlement_method, NULL::text
		  FROM accounts_receivable WHERE tenant_id=$1
		) AS all_accounts `+where+`
		ORDER BY due_date ASC, id ASC LIMIT 500
	`, au.TenantID)
	if err != nil {
		accountFailure(w, r, http.StatusInternalServerError, "Não foi possível consultar contas")
		return
	}
	defer rows.Close()
	items := make([]financeAccount, 0)
	for rows.Next() {
		var item financeAccount
		var amount string
		if err := rows.Scan(&item.ID, &item.Kind, &item.Description, &amount, &item.DueDate,
			&item.Status, &item.SettledAt, &item.SettlementMethod, &item.PurchaseID); err != nil {
			accountFailure(w, r, http.StatusInternalServerError, "Não foi possível ler contas")
			return
		}
		money, err := platform.ParseMoney(amount)
		if err != nil {
			accountFailure(w, r, http.StatusInternalServerError, "Valor de conta inválido no banco")
			return
		}
		item.Amount = money
		items = append(items, item)
	}
	if rows.Err() != nil {
		accountFailure(w, r, http.StatusInternalServerError, "Erro ao consultar contas")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "truncated": len(items) == 500})
}

// Trends uses the underlying tenant-scoped ledger, not sampled UI rows.
// Values show movements recorded by the system and are not bank balances.
func (h *FinanceAccountsHandler) Trends(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		accountFailure(w, r, 401, "Sessão inválida")
		return
	}
	days := 14
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || (n != 7 && n != 14 && n != 30 && n != 90) {
			accountFailure(w, r, 422, "Escolha 7, 14, 30 ou 90 dias")
			return
		}
		days = n
	}
	rows, err := h.pool.Query(r.Context(), `
		WITH dates AS (
		  SELECT d::date AS day FROM generate_series(
		    (now() AT TIME ZONE 'America/Sao_Paulo')::date - ($2::int - 1),
		    (now() AT TIME ZONE 'America/Sao_Paulo')::date,
		    interval '1 day'
		  ) d
		), totals AS (
		  SELECT (created_at AT TIME ZONE 'America/Sao_Paulo')::date AS day,
		    COALESCE(SUM(amount_net) FILTER (WHERE entry_type IN ('sale','sale_cancel','revenue')),0) AS inflow,
		    COALESCE(SUM(amount_net) FILTER (WHERE entry_type IN ('expense','return_refund')),0) AS outflow
		  FROM ledger_entries
		  WHERE tenant_id=$1
		    AND created_at >= ((now() AT TIME ZONE 'America/Sao_Paulo')::date - ($2::int - 1))::timestamp AT TIME ZONE 'America/Sao_Paulo'
		  GROUP BY 1
		)
		SELECT dates.day::text,
		  COALESCE(t.inflow,0)::text, COALESCE(t.outflow,0)::text
		FROM dates LEFT JOIN totals t ON dates.day=t.day ORDER BY dates.day
	`, au.TenantID, days)
	if err != nil {
		accountFailure(w, r, 500, "Não foi possível calcular a evolução")
		return
	}
	defer rows.Close()
	type point struct {
		Date    string         `json:"date"`
		Inflow  platform.Money `json:"inflow"`
		Outflow platform.Money `json:"outflow"`
	}
	items := make([]point, 0, days)
	for rows.Next() {
		var p point
		var incoming, outgoing string
		if err := rows.Scan(&p.Date, &incoming, &outgoing); err != nil {
			accountFailure(w, r, 500, "Não foi possível ler a evolução")
			return
		}
		inMoney, errA := platform.ParseMoney(incoming)
		outMoney, errB := platform.ParseMoney(outgoing)
		if errA != nil || errB != nil {
			accountFailure(w, r, 500, "Valor financeiro inválido")
			return
		}
		p.Inflow = inMoney
		p.Outflow = outMoney
		items = append(items, p)
	}
	if rows.Err() != nil {
		accountFailure(w, r, 500, "Não foi possível consultar a evolução")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "items": items, "basis": "ledger_recorded_movements"})
}

type accountCreate struct {
	Kind        string         `json:"kind"`
	Description string         `json:"description"`
	Amount      platform.Money `json:"amount"`
	DueDate     string         `json:"due_date"`
}

func (h *FinanceAccountsHandler) Create(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		accountFailure(w, r, 401, "Sessão inválida")
		return
	}
	key, ok := accountKey(r)
	if !ok {
		accountFailure(w, r, 422, "Informe uma Idempotency-Key UUID válida")
		return
	}
	var req accountCreate
	if err := readJSON(w, r, &req); err != nil {
		accountFailure(w, r, 400, "Dados da conta inválidos")
		return
	}
	table, _, ok := accountTable(req.Kind)
	req.Description = strings.TrimSpace(req.Description)
	if !ok || len([]rune(req.Description)) < 3 || len([]rune(req.Description)) > 250 || req.Amount <= 0 {
		accountFailure(w, r, 422, "Informe tipo, descrição e valor positivo válidos")
		return
	}
	date, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil || date.Format("2006-01-02") != req.DueDate {
		accountFailure(w, r, 422, "Vencimento inválido")
		return
	}
	data := fmt.Sprintf("%s|%s|%s|%s|%s", au.UserID, req.Kind, req.Description, req.Amount.DBString(), req.DueDate)
	fingerprint := sha256.Sum256([]byte(data))
	hash := hex.EncodeToString(fingerprint[:])
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		accountFailure(w, r, 503, "Banco indisponível")
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO `+table+`
		(tenant_id, description, amount, due_date, created_by_user_id, creation_key, request_hash)
		VALUES ($1,$2,$3,$4::date,$5,$6,$7)
		ON CONFLICT (tenant_id, creation_key) WHERE creation_key IS NOT NULL DO NOTHING
		RETURNING id::text`,
		au.TenantID, req.Description, req.Amount.DBString(), req.DueDate, au.UserID, key, hash).Scan(&id)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		var savedHash string
		err = tx.QueryRow(r.Context(), `SELECT id::text, request_hash FROM `+table+` WHERE tenant_id=$1 AND creation_key=$2`, au.TenantID, key).Scan(&id, &savedHash)
		if err == nil && savedHash != hash {
			accountFailure(w, r, 409, "Chave de repetição já usada para outra conta")
			return
		}
	}
	if err != nil {
		accountFailure(w, r, 500, "Não foi possível registrar a conta")
		return
	}
	if created {
		requestID, ip, agent := audit.RequestContext(r)
		if err := h.audit.RecordTx(r.Context(), tx, audit.Event{
			TenantID: au.TenantID, ActorUserID: au.UserID, Action: "finance.account.create",
			ResourceType: table, ResourceID: id, RequestID: requestID, IP: ip, UserAgent: agent,
			Metadata: map[string]any{"kind": req.Kind, "amount": req.Amount.DBString()},
		}); err != nil {
			accountFailure(w, r, 500, "Não foi possível auditar a conta")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		accountFailure(w, r, 500, "Não foi possível confirmar a conta")
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"id": id, "replayed": !created})
}

type accountSettle struct {
	Method string `json:"method"`
	Note   string `json:"note"`
}

func (h *FinanceAccountsHandler) Settle(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		accountFailure(w, r, 401, "Sessão inválida")
		return
	}
	key, ok := accountKey(r)
	if !ok {
		accountFailure(w, r, 422, "Informe uma Idempotency-Key UUID válida")
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		accountFailure(w, r, 422, "Conta inválida")
		return
	}
	kind := chi.URLParam(r, "kind")
	table, settled, ok := accountTable(kind)
	if !ok {
		accountFailure(w, r, 422, "Tipo de conta inválido")
		return
	}
	var req accountSettle
	if err := readJSON(w, r, &req); err != nil {
		accountFailure(w, r, 400, "Dados da baixa inválidos")
		return
	}
	switch req.Method {
	case "cash", "pix", "debit", "credit", "transfer", "other":
	default:
		accountFailure(w, r, 422, "Forma de pagamento inválida")
		return
	}
	req.Note = strings.TrimSpace(req.Note)
	if len([]rune(req.Note)) > 500 {
		accountFailure(w, r, 422, "Observação muito longa")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		accountFailure(w, r, 503, "Banco indisponível")
		return
	}
	defer tx.Rollback(r.Context())
	var amount, status string
	var savedKey, method *string
	err = tx.QueryRow(r.Context(), `SELECT amount::text, status, settlement_key::text, settlement_method
		FROM `+table+` WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, au.TenantID, id).Scan(&amount, &status, &savedKey, &method)
	if errors.Is(err, pgx.ErrNoRows) {
		accountFailure(w, r, 404, "Conta não encontrada")
		return
	}
	if err != nil {
		accountFailure(w, r, 500, "Não foi possível consultar a conta")
		return
	}
	if status == settled && savedKey != nil && *savedKey == key && method != nil && *method == req.Method {
		writeJSON(w, 200, map[string]any{"id": id, "status": settled, "replayed": true})
		return
	}
	if status != "open" {
		accountFailure(w, r, 409, "Conta já baixada ou cancelada")
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE `+table+`
		SET status=$3, settled_at=now(), settled_by_user_id=$4, settlement_key=$5,
		    settlement_method=$6, settlement_note=$7
		WHERE tenant_id=$1 AND id=$2 AND status='open'`,
		au.TenantID, id, settled, au.UserID, key, req.Method, req.Note)
	if err != nil || tag.RowsAffected() != 1 {
		accountFailure(w, r, 409, "Baixa não confirmada")
		return
	}
	money, err := platform.ParseMoney(amount)
	if err != nil {
		accountFailure(w, r, 500, "Valor de conta inválido")
		return
	}
	entryType := "revenue"
	if kind == "payable" {
		entryType = "expense"
		money = money.Neg()
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO ledger_entries(tenant_id, entry_type,
		amount_net, amount_gross, notes, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		au.TenantID, entryType, money.DBString(), money.DBString(),
		"Baixa de conta "+kind+" "+id, au.UserID)
	if err != nil {
		accountFailure(w, r, 500, "Não foi possível registrar o lançamento")
		return
	}
	requestID, ip, agent := audit.RequestContext(r)
	if err := h.audit.RecordTx(r.Context(), tx, audit.Event{
		TenantID: au.TenantID, ActorUserID: au.UserID, Action: "finance.account.settle",
		ResourceType: table, ResourceID: id, RequestID: requestID, IP: ip, UserAgent: agent,
		Metadata: map[string]any{"method": req.Method, "amount": amount},
	}); err != nil {
		accountFailure(w, r, 500, "Não foi possível auditar a baixa")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		accountFailure(w, r, 500, "Não foi possível confirmar a baixa")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "status": settled, "replayed": false})
}
