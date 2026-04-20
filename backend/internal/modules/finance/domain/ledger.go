package domain

type LedgerEntry struct {
	ID              string   `json:"id"`
	EntryType       string   `json:"entry_type"`
	SaleID          *string  `json:"sale_id"`
	CashSessionID   *string  `json:"cash_session_id"`
	AmountGross     float64  `json:"amount_gross"`
	AmountDiscount  float64  `json:"amount_discount"`
	AmountNet       float64  `json:"amount_net"`
	ProfitEstimated float64  `json:"profit_estimated"`
	Notes           *string  `json:"notes"`
	CreatedAt       string   `json:"created_at"`
}
