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
	Protocol    string    `json:"protocol,omitempty"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
	ResponseXML []byte    `json:"-"`
}

func (r NFCeCancellationRemoteResult) Registered() bool {
	return r.StatusCode == 135 && r.Protocol != "" && !r.RegisteredAt.IsZero()
}
