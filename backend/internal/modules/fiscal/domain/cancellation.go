package domain

import "time"

const (
	NFCeCancellationEventType = "110111"

	NFCeEventStatusPrepared  = "prepared"
	NFCeEventStatusSigned    = "signed"
	NFCeEventStatusSubmitted = "submitted"
	NFCeEventStatusRegistered = "registered"
	NFCeEventStatusRejected  = "rejected"
)

type NFCeCancellationDraft struct {
	Environment           string    `json:"environment"`
	IssuerUF              string    `json:"issuer_uf"`
	IssuerCNPJ            string    `json:"issuer_cnpj"`
	AccessKey             string    `json:"access_key"`
	AuthorizationProtocol string    `json:"authorization_protocol"`
	EventTime             time.Time `json:"event_time"`
	Sequence              int       `json:"sequence"`
	Justification         string    `json:"justification"`
}

type NFCeCancellationEvent struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	InvoiceID     string     `json:"invoice_id"`
	EventType     string     `json:"event_type"`
	Sequence      int        `json:"sequence"`
	EventID       string     `json:"event_id"`
	Environment   string     `json:"environment"`
	Status        string     `json:"status"`
	Justification string     `json:"justification"`
	SignedSHA256  *string    `json:"signed_sha256,omitempty"`
	ResponseSHA256 *string   `json:"response_sha256,omitempty"`
	StatusCode    *int       `json:"status_code,omitempty"`
	Reason        *string    `json:"reason,omitempty"`
	Protocol      *string    `json:"protocol,omitempty"`
	RegisteredAt  *time.Time `json:"registered_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type NFCeCancellationRemoteResult struct {
	AccessKey   string    `json:"access_key"`
	EventID     string    `json:"event_id"`
	Sequence    int       `json:"sequence"`
	StatusCode  int       `json:"status_code"`
	Reason      string    `json:"reason"`
	FinalStatus string    `json:"final_status,omitempty"`
	Protocol    string    `json:"protocol,omitempty"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
	ResponseXML []byte    `json:"-"`
}

func (r NFCeCancellationRemoteResult) Registered() bool {
	return r.FinalStatus == NFCeEventStatusRegistered &&
		r.StatusCode == 135 && r.Protocol != "" && !r.RegisteredAt.IsZero()
}

func (r NFCeCancellationRemoteResult) Rejected() bool {
	return r.FinalStatus == NFCeEventStatusRejected
}

func (r NFCeCancellationRemoteResult) Pending() bool {
	return r.FinalStatus == ""
}
