package domain

import "github.com/example/sistemaemgo/internal/platform"

type PaymentRecord struct {
	ID                   string          `json:"id"`
	SaleID               string          `json:"sale_id"`
	Method               string          `json:"method"`
	Amount               platform.Money  `json:"amount"`
	Provider             *string         `json:"provider,omitempty"`
	TransactionRef       *string         `json:"transaction_ref,omitempty"`
	AuthorizationCode    *string         `json:"authorization_code,omitempty"`
	Installments         int             `json:"installments"`
	ReconciliationStatus string          `json:"reconciliation_status"`
	ReconciledAmount     *platform.Money `json:"reconciled_amount,omitempty"`
	ReconciledFee        *platform.Money `json:"reconciled_fee,omitempty"`
	ReconciledAt         *string         `json:"reconciled_at,omitempty"`
	ReconciliationNotes  *string         `json:"reconciliation_notes,omitempty"`
	CreatedAt            string          `json:"created_at"`
}

type PaymentReconciliation struct {
	ID             string         `json:"id"`
	PaymentID      string         `json:"payment_id"`
	ExpectedAmount platform.Money `json:"expected_amount"`
	ReceivedAmount platform.Money `json:"received_amount"`
	FeeAmount      platform.Money `json:"fee_amount"`
	NetAmount      platform.Money `json:"net_amount"`
	Difference     platform.Money `json:"difference_amount"`
	Status         string         `json:"status"`
	Provider       *string        `json:"provider,omitempty"`
	ExternalRef    *string        `json:"external_ref,omitempty"`
	Notes          *string        `json:"notes,omitempty"`
	CreatedBy      string         `json:"created_by_user_id"`
	CreatedAt      string         `json:"created_at"`
}

type PaymentReconciliationAdjustment struct {
	ID                     string         `json:"id"`
	PaymentID              string         `json:"payment_id"`
	PreviousReceivedAmount platform.Money `json:"previous_received_amount"`
	PreviousFeeAmount      platform.Money `json:"previous_fee_amount"`
	NewReceivedAmount      platform.Money `json:"new_received_amount"`
	NewFeeAmount           platform.Money `json:"new_fee_amount"`
	Difference             platform.Money `json:"difference_amount"`
	Status                 string         `json:"status"`
	Notes                  *string        `json:"notes,omitempty"`
	CreatedBy              string         `json:"created_by_user_id"`
	CreatedAt              string         `json:"created_at"`
}

type ReturnRefundSummary struct {
	ReturnID      string         `json:"return_id"`
	SaleID        string         `json:"sale_id"`
	Kind          string         `json:"kind"`
	Reason        string         `json:"reason"`
	RefundDue     platform.Money `json:"refund_due"`
	SettledAmount platform.Money `json:"settled_amount"`
	Remaining     platform.Money `json:"remaining_amount"`
	Status        string         `json:"status"`
	CreatedAt     string         `json:"created_at"`
}

type ReturnRefund struct {
	ID            string         `json:"id"`
	ReturnID      string         `json:"return_id"`
	SaleID        string         `json:"sale_id"`
	Method        string         `json:"method"`
	Amount        platform.Money `json:"amount"`
	Provider      *string        `json:"provider,omitempty"`
	ExternalRef   *string        `json:"external_ref,omitempty"`
	CashSessionID *string        `json:"cash_session_id,omitempty"`
	Notes         *string        `json:"notes,omitempty"`
	CreatedBy     string         `json:"created_by_user_id"`
	CreatedAt     string         `json:"created_at"`
}
