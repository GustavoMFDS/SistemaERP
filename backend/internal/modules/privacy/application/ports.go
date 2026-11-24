package application

import (
	"context"

	"github.com/example/sistemaemgo/internal/modules/audit"
	privacy "github.com/example/sistemaemgo/internal/modules/privacy/domain"
)

type Repository interface {
	CreateRequest(ctx context.Context, tenantID, actorUserID, requestID string, req CreateRequest) (string, error)
	ListRequests(ctx context.Context, tenantID string, limit, offset int) ([]privacy.DataSubjectRequest, error)
	GetRequest(ctx context.Context, tenantID, id string) (privacy.DataSubjectRequest, error)
	UpdateRequestStatus(ctx context.Context, tenantID, id, previousStatus, status string, notes *string, event audit.Event) error
	ExportSubjectData(ctx context.Context, tenantID, subjectType, subjectID string) (map[string]any, error)
	AnonymizeSubject(ctx context.Context, tenantID, subjectType, subjectID string, event audit.Event) error
	BlockSubject(ctx context.Context, tenantID, subjectType, subjectID string, event audit.Event) error
	RecordConsent(ctx context.Context, tenantID, requestID string, req ConsentCreateRequest) (string, error)
	ListConsents(ctx context.Context, tenantID string, limit, offset int) ([]privacy.ConsentRecord, error)
	RevokeConsent(ctx context.Context, tenantID, id string) error
}
