package domain

import "time"

const (
	NFCeStatusReserved   = "reserved"
	NFCeStatusSigned     = "signed"
	NFCeStatusSubmitted  = "submitted"
	NFCeStatusAuthorized = "authorized"
	NFCeStatusRejected   = "rejected"
	NFCeStatusCancelled  = "cancelled"
)

type NFCeAuthorizationResult struct {
	Status           string
	AccessKey        string
	Protocol         string
	AuthorizedAt     time.Time
	RejectionCode    string
	RejectionMessage string
}

func (r NFCeAuthorizationResult) IsAuthorized() bool {
	return r.Status == NFCeStatusAuthorized
}

func (r NFCeAuthorizationResult) IsRejected() bool {
	return r.Status == NFCeStatusRejected
}
