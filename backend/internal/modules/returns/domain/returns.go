package domain

import "github.com/example/sistemaemgo/internal/platform"

type Kind string

const (
	KindReturn   Kind = "return"
	KindExchange Kind = "exchange"
)

type SaleReturn struct {
	ID                string         `json:"id"`
	SaleID            string         `json:"sale_id"`
	Kind              Kind           `json:"kind"`
	Reason            string         `json:"reason"`
	RefundDue         platform.Money `json:"refund_due"`
	ReplacementSaleID *string        `json:"replacement_sale_id,omitempty"`
	CreatedByUserID   string         `json:"created_by_user_id"`
	CreatedAt         string         `json:"created_at"`
}

type Item struct {
	ID          string            `json:"id"`
	ReturnID    string            `json:"return_id"`
	SaleItemID  string            `json:"sale_item_id"`
	ProductID   string            `json:"product_id"`
	Qty         platform.Quantity `json:"qty"`
	Restock     bool              `json:"restock"`
	RefundValue platform.Money    `json:"refund_value"`
	CreatedAt   string            `json:"created_at"`
}
