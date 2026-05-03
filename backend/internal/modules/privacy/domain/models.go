package domain

type DataSubjectRequest struct {
	ID              string  `json:"id"`
	TenantID        string  `json:"tenant_id,omitempty"`
	SubjectType     string  `json:"subject_type"`
	SubjectID       *string `json:"subject_id,omitempty"`
	RequesterEmail  *string `json:"requester_email,omitempty"`
	RequestType     string  `json:"request_type"`
	Status          string  `json:"status"`
	Notes           *string `json:"notes,omitempty"`
	RequestedAt     string  `json:"requested_at"`
	ResolvedAt      *string `json:"resolved_at,omitempty"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	RequestID       *string `json:"request_id,omitempty"`
}

type ConsentRecord struct {
	ID                 string  `json:"id"`
	TenantID           string  `json:"tenant_id,omitempty"`
	SubjectType        string  `json:"subject_type"`
	SubjectID          *string `json:"subject_id,omitempty"`
	Purpose            string  `json:"purpose"`
	ConsentTextVersion string  `json:"consent_text_version"`
	ConsentedAt        string  `json:"consented_at"`
	Source             string  `json:"source"`
	WithdrawnAt        *string `json:"withdrawn_at,omitempty"`
	RequestID          *string `json:"request_id,omitempty"`
}
