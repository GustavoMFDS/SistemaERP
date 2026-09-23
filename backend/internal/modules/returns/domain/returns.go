package domain

import "github.com/example/sistemaemgo/internal/platform"

type Return struct {
	ID             string         `json:"id"`
	SaleID         string         `json:"sale_id"`
	Reason         string         `json:"reason"`
	TotalAmount    platform.Money `json:"total_amount"`
	RecoveredCost  platform.Money `json:"recovered_cost"`
	RefundedAmount platform.Money `json:"refunded_amount"`
	CreatedBy      string         `json:"created_by_user_id"`
	CreatedAt      string         `json:"created_at"`
}

type ReturnItem struct {
	ID            string            `json:"id"`
	ReturnID      string            `json:"return_id"`
	SaleItemID    string            `json:"sale_item_id"`
	ProductID     string            `json:"product_id"`
	Qty           platform.Quantity `json:"qty"`
	Restock       bool              `json:"restock"`
	Amount        platform.Money    `json:"amount"`
	RecoveredCost platform.Money    `json:"recovered_cost"`
}

type Refund struct {
	ID                string         `json:"id"`
	ReturnID          string         `json:"return_id"`
	SaleID            string         `json:"sale_id"`
	Method            string         `json:"method"`
	Amount            platform.Money `json:"amount"`
	ExternalReference *string        `json:"external_reference"`
	Notes             *string        `json:"notes"`
	CreatedBy         string         `json:"created_by_user_id"`
	CreatedAt         string         `json:"created_at"`
}

type ReturnedAggregate struct {
	Qty    platform.Quantity
	Amount platform.Money
}
