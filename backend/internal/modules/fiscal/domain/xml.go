package domain

type XMLFile struct {
	ID        string `json:"id"`
	InvoiceID string `json:"invoice_id"`
	FileName  string `json:"file_name"`
	SHA256    string `json:"sha256"`
	CreatedAt string `json:"created_at"`
}
