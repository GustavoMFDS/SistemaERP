package domain

import "time"

const (
	NFCeInutilizationStatusSigned     = "signed"
	NFCeInutilizationStatusSubmitted  = "submitted"
	NFCeInutilizationStatusRegistered = "registered"
	NFCeInutilizationStatusRejected   = "rejected"
)

type NFCeInutilizationDraft struct {
	Environment   string `json:"environment"`
	IssuerUF      string `json:"issuer_uf"`
	IssuerCNPJ    string `json:"issuer_cnpj"`
	Year          int    `json:"year"`
	Series        int    `json:"series"`
	StartNumber   int64  `json:"start_number"`
	EndNumber     int64  `json:"end_number"`
	Justification string `json:"justification"`
}

type NFCeInutilization struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Environment    string     `json:"environment"`
	Year           int        `json:"year"`
	Model          int        `json:"model"`
	Series         int        `json:"series"`
	StartNumber    int64      `json:"start_number"`
	EndNumber      int64      `json:"end_number"`
	RequestID      string     `json:"request_id"`
	Status         string     `json:"status"`
	Justification  string     `json:"justification"`
	SignedSHA256   *string    `json:"signed_sha256,omitempty"`
	ResponseSHA256 *string    `json:"response_sha256,omitempty"`
	StatusCode     *int       `json:"status_code,omitempty"`
	Reason         *string    `json:"reason,omitempty"`
	Protocol       *string    `json:"protocol,omitempty"`
	RegisteredAt   *time.Time `json:"registered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type NFCeInutilizationRemoteResult struct {
	RequestID    string    `json:"request_id"`
	StatusCode   int       `json:"status_code"`
	Reason       string    `json:"reason"`
	FinalStatus  string    `json:"final_status,omitempty"`
	Protocol     string    `json:"protocol,omitempty"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
	ResponseXML  []byte    `json:"-"`
}

func (r NFCeInutilizationRemoteResult) Registered() bool {
	return r.FinalStatus == NFCeInutilizationStatusRegistered &&
		r.StatusCode == 102 && r.Protocol != "" && !r.RegisteredAt.IsZero()
}

func (r NFCeInutilizationRemoteResult) Rejected() bool {
	return r.FinalStatus == NFCeInutilizationStatusRejected
}

func (r NFCeInutilizationRemoteResult) Pending() bool {
	return r.FinalStatus == ""
}
