package application

import (
	"context"

	privacy "github.com/example/sistemaemgo/internal/modules/privacy/domain"
)

type Repository interface {
	SubjectBelongsToTenant(ctx context.Context, tenantID, subjectType, subjectID string) (bool, error)
	CreateRequest(ctx context.Context, tenantID, actorUserID, requestID string, req CreateRequest) (string, error)
	ListRequests(ctx context.Context, tenantID string, limit, offset int) ([]privacy.DataSubjectRequest, error)
	GetRequest(ctx context.Context, tenantID, id string) (privacy.DataSubjectRequest, error)
	UpdateRequestStatus(ctx context.Context, tenantID, id, status string, notes *string) error
	ExportSubjectData(ctx context.Context, tenantID, subjectType, subjectID string) (map[string]any, error)
	AnonymizeSubject(ctx context.Context, tenantID, subjectType, subjectID string) error
	BlockSubject(ctx context.Context, tenantID, subjectType, subjectID string) error
	RecordConsent(ctx context.Context, tenantID, requestID string, req ConsentCreateRequest) (string, error)
	ListConsents(ctx context.Context, tenantID string, limit, offset int) ([]privacy.ConsentRecord, error)
	RevokeConsent(ctx context.Context, tenantID, id string) error
}
