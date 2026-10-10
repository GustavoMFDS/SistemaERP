package domain

import "time"

type NFCeRemoteOutcome struct {
	AccessKey   string    `json:"access_key"`
	StatusCode  int       `json:"status_code"`
	Reason      string    `json:"reason"`
	FinalStatus string    `json:"final_status,omitempty"`
	Protocol    string    `json:"protocol,omitempty"`
	ReceivedAt  time.Time `json:"received_at,omitempty"`
	ProtocolXML []byte `json:"-"`
}

func (o NFCeRemoteOutcome) Authorized() bool {
	return o.FinalStatus == NFCeStatusAuthorized
}

func (o NFCeRemoteOutcome) Rejected() bool {
	return o.FinalStatus == NFCeStatusRejected
}

func (o NFCeRemoteOutcome) Pending() bool {
	return o.FinalStatus == ""
}
