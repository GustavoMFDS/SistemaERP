package domain

type NFCeReservation struct {
	InvoiceID      string `json:"invoice_id"`
	SaleID         string `json:"sale_id"`
	Status         string `json:"status"`
	Model          int    `json:"model"`
	Series         int    `json:"series"`
	DocumentNumber int64  `json:"document_number"`
	Environment    string `json:"environment"`
	AccessKey      string `json:"access_key"`
	EmissionType   int    `json:"emission_type"`
	NumericCode    string `json:"numeric_code"`
	CheckDigit     int    `json:"access_key_check_digit"`
	IssuedAt       string `json:"issued_at"`
}

type NFCeReservationContext struct {
	Issuer NFCeIssuerProfile
	Config NFCeConfig
}
