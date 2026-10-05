package domain

import "time"

type NFCeReservation struct {
	InvoiceID             string     `json:"invoice_id"`
	SaleID                string     `json:"sale_id"`
	Status                string     `json:"status"`
	Model                 int        `json:"model"`
	Series                int        `json:"series"`
	DocumentNumber        int64      `json:"document_number"`
	Environment           string     `json:"environment"`
	AccessKey             string     `json:"access_key"`
	EmissionType          int        `json:"emission_type"`
	NumericCode           string     `json:"numeric_code"`
	CheckDigit            int        `json:"access_key_check_digit"`
	IssuedAt                time.Time  `json:"issued_at"`
	ContingencyStartedAt    *time.Time `json:"contingency_started_at,omitempty"`
	ContingencyJustification *string    `json:"contingency_justification,omitempty"`
	AuthorizationProtocol   *string    `json:"authorization_protocol,omitempty"`
	AuthorizedAt          *time.Time `json:"authorized_at,omitempty"`
}

type NFCeReservationContext struct {
	Issuer NFCeIssuerProfile
	Config NFCeConfig
}
