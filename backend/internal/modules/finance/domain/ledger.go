package domain

import "github.com/example/sistemaemgo/internal/platform"

type LedgerEntry struct {
	ID              string         `json:"id"`
	EntryType       string         `json:"entry_type"`
	SaleID          *string        `json:"sale_id"`
	CashSessionID   *string        `json:"cash_session_id"`
	AmountGross     platform.Money `json:"amount_gross"`
	AmountDiscount  platform.Money `json:"amount_discount"`
	AmountNet       platform.Money `json:"amount_net"`
	ProfitEstimated platform.Money `json:"profit_estimated"`
	Notes           *string        `json:"notes"`
	CreatedAt       string         `json:"created_at"`
}
