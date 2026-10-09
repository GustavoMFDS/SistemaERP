package domain

import "github.com/example/sistemaemgo/internal/platform"

// OwnerOverview reports ledger figures scoped to one tenant and posting period.
// Sales totals include cancellations but not returns; estimates do not deduct
// payment fees, taxes, operating expenses or the cost impact of returned items.
type OwnerOverview struct {
	SalesAfterCancellations platform.Money `json:"sales_after_cancellations"`
	EstimatedGrossProfit    platform.Money `json:"estimated_gross_profit"`
	RefundsRecorded         platform.Money `json:"refunds_recorded"`
	SalesCount              int64          `json:"sales_count"`
	CancelledCount          int64          `json:"cancelled_count"`
}
