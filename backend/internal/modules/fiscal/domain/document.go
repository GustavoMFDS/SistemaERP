package domain

import "github.com/example/sistemaemgo/internal/platform"

type NFCeDocumentItem struct {
	Number        int                       `json:"number"`
	SaleItemID    string                    `json:"sale_item_id"`
	ProductID     string                    `json:"product_id"`
	Code          string                    `json:"code"`
	Description   string                    `json:"description"`
	Unit          string                    `json:"unit"`
	NCM           string                    `json:"ncm"`
	CEST          *string                   `json:"cest,omitempty"`
	CFOP          string                    `json:"cfop"`
	Quantity      platform.Quantity         `json:"quantity"`
	UnitPrice     platform.Money            `json:"unit_price"`
	GrossValue    platform.Money            `json:"gross_value"`
	DiscountValue platform.Money            `json:"discount_value"`
	NetValue      platform.Money            `json:"net_value"`
	Tax           InvoiceItemTaxCalculation `json:"tax"`
}

type NFCeDocumentPayment struct {
	Method            string         `json:"method"`
	Amount            platform.Money `json:"amount"`
	Provider          *string        `json:"provider,omitempty"`
	TransactionRef    *string        `json:"transaction_ref,omitempty"`
	AuthorizationCode *string        `json:"authorization_code,omitempty"`
	Installments      int            `json:"installments"`
}

type NFCeDocumentDraft struct {
	Reservation     NFCeReservation       `json:"reservation"`
	Issuer          NFCeIssuerProfile     `json:"issuer"`
	CustomerID      *string               `json:"customer_id,omitempty"`
	CommercialTotal platform.Money        `json:"commercial_total"`
	Items           []NFCeDocumentItem    `json:"items"`
	Payments        []NFCeDocumentPayment `json:"payments"`
}
