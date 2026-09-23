package domain

import "github.com/example/sistemaemgo/internal/platform"

type PurchaseStatus string

const (
	PurchaseOrdered           PurchaseStatus = "ordered"
	PurchasePartiallyReceived PurchaseStatus = "partially_received"
	PurchaseReceived          PurchaseStatus = "received"
	PurchaseCancelled         PurchaseStatus = "cancelled"
)

type Supplier struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Document    *string `json:"document"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	ContactName *string `json:"contact_name"`
	Notes       *string `json:"notes"`
	Active      bool    `json:"active"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type Purchase struct {
	ID             string         `json:"id"`
	SupplierID     string         `json:"supplier_id"`
	SupplierName   string         `json:"supplier_name"`
	Status         PurchaseStatus `json:"status"`
	InvoiceNumber  *string        `json:"invoice_number"`
	PaymentDueDate *string        `json:"payment_due_date"`
	Total          platform.Money `json:"total"`
	Notes          *string        `json:"notes"`
	CreatedBy      string         `json:"created_by_user_id"`
	OrderedAt      string         `json:"ordered_at"`
	ReceivedAt     *string        `json:"received_at"`
	CancelledAt    *string        `json:"cancelled_at"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type PurchaseItem struct {
	ID          string            `json:"id"`
	PurchaseID  string            `json:"purchase_id"`
	ProductID   string            `json:"product_id"`
	ProductSKU  string            `json:"product_sku"`
	ProductName string            `json:"product_name"`
	QtyOrdered  platform.Quantity `json:"qty_ordered"`
	QtyReceived platform.Quantity `json:"qty_received"`
	UnitCost    platform.Money    `json:"unit_cost"`
	LineTotal   platform.Money    `json:"line_total"`
}

type Receipt struct {
	ID           string  `json:"id"`
	PurchaseID   string  `json:"purchase_id"`
	ReceivedBy   string  `json:"received_by_user_id"`
	Notes        *string `json:"notes"`
	ReceivedAt   string  `json:"received_at"`
}

type ReceiptItem struct {
	ID             string            `json:"id"`
	ReceiptID      string            `json:"receipt_id"`
	PurchaseItemID string            `json:"purchase_item_id"`
	ProductID      string            `json:"product_id"`
	Qty            platform.Quantity `json:"qty"`
	UnitCost       platform.Money    `json:"unit_cost"`
}
