package application

import (
	"context"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
	privacy "github.com/example/sistemaemgo/internal/modules/privacy/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateRequestValidatesAndUsesTenantContext(t *testing.T) {
	repo := &fakePrivacyRepo{}
	svc := newPrivacyService(repo)
	subjectID := "11111111-1111-1111-1111-111111111111"
	id, err := svc.CreateRequest(context.Background(), "tenant-1", "actor-1", "req-1", CreateRequest{
		SubjectType: "customer",
		SubjectID:   &subjectID,
		RequestType: "export",
	})
	if err != nil {
		t.Fatalf("CreateRequest returned error: %v", err)
	}
	if id == "" || repo.createdTenant != "tenant-1" {
		t.Fatalf("expected tenant-scoped create, id=%q tenant=%q", id, repo.createdTenant)
	}
}

func TestCreateRequestRejectsSubjectOutsideTenant(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	allowed := false
	svc := newPrivacyService(&fakePrivacyRepo{subjectAllowed: &allowed})

	_, err := svc.CreateRequest(context.Background(), "tenant-1", "actor-1", "req-1", CreateRequest{
		SubjectType: "customer",
		SubjectID:   &subjectID,
		RequestType: "export",
	})
	if err != common.ErrNotFound {
		t.Fatalf("expected tenant subject to be rejected, got %v", err)
	}
}

func TestConsentRequiresSubjectInsideTenant(t *testing.T) {
	svc := newPrivacyService(&fakePrivacyRepo{})
	_, err := svc.RecordConsent(context.Background(), "tenant-1", "req-1", ConsentCreateRequest{
		SubjectType:        "customer",
		Purpose:            "marketing",
		ConsentTextVersion: "v1",
		Source:             "web",
	})
	if err != common.ErrValidation {
		t.Fatalf("expected missing subject validation error, got %v", err)
	}

	subjectID := "11111111-1111-1111-1111-111111111111"
	allowed := false
	svc = newPrivacyService(&fakePrivacyRepo{subjectAllowed: &allowed})
	_, err = svc.RecordConsent(context.Background(), "tenant-1", "req-1", ConsentCreateRequest{
		SubjectType:        "customer",
		SubjectID:          &subjectID,
		Purpose:            "marketing",
		ConsentTextVersion: "v1",
		Source:             "web",
	})
	if err != common.ErrNotFound {
		t.Fatalf("expected cross-tenant consent subject rejection, got %v", err)
	}
}

func TestCreateRequestRejectsUnsupportedCustomerBlocking(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	svc := newPrivacyService(&fakePrivacyRepo{})

	_, err := svc.CreateRequest(context.Background(), "tenant-1", "actor-1", "req-1", CreateRequest{
		SubjectType: "customer",
		SubjectID:   &subjectID,
		RequestType: "blocking",
	})
	if err != common.ErrValidation {
		t.Fatalf("customer blocking request must be rejected, got %v", err)
	}
}

func TestCreateRequestRejectsInvalidInput(t *testing.T) {
	svc := newPrivacyService(&fakePrivacyRepo{})
	_, err := svc.CreateRequest(context.Background(), "tenant-1", "actor-1", "req-1", CreateRequest{
		SubjectType: "customer",
		RequestType: "export",
	})
	if err != common.ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestExportAndAnonymizeUseStoredRequestSubject(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	repo := &fakePrivacyRepo{request: privacy.DataSubjectRequest{
		ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1",
		SubjectType: "customer", SubjectID: &subjectID, RequestType: "export", Status: "open",
	}}
	svc := newPrivacyService(repo)

	data, err := svc.ExportSubjectData(context.Background(), "tenant-1", repo.request.ID)
	if err != nil {
		t.Fatalf("ExportSubjectData returned error: %v", err)
	}
	if data["subject_type"] != "customer" || repo.exportedTenant != "tenant-1" {
		t.Fatalf("unexpected export data=%v tenant=%q", data, repo.exportedTenant)
	}

	repo.request.RequestType = "anonymization"
	repo.request.Status = "in_progress"
	if err := svc.AnonymizeSubject(context.Background(), "tenant-1", "actor-1", repo.request.ID); err != nil {
		t.Fatalf("AnonymizeSubject returned error: %v", err)
	}
	if !repo.anonymized {
		t.Fatalf("expected subject anonymization")
	}
	if repo.request.Status != "completed" {
		t.Fatalf("anonymization should complete DSR atomically, got status %q", repo.request.Status)
	}
}

func TestSubjectActionsRequireMatchingRequestTypeAndReviewState(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	repo := &fakePrivacyRepo{request: privacy.DataSubjectRequest{
		ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1",
		SubjectType: "customer", SubjectID: &subjectID, RequestType: "export", Status: "open",
	}}
	svc := newPrivacyService(repo)

	if err := svc.AnonymizeSubject(context.Background(), "tenant-1", "actor-1", repo.request.ID); err != common.ErrConflict {
		t.Fatalf("export request must not anonymize subject, got %v", err)
	}
	repo.request.RequestType = "anonymization"
	if err := svc.AnonymizeSubject(context.Background(), "tenant-1", "actor-1", repo.request.ID); err != common.ErrConflict {
		t.Fatalf("open anonymization request must enter in_progress first, got %v", err)
	}

	repo.request.SubjectType = "user"
	repo.request.RequestType = "blocking"
	repo.request.Status = "completed"
	if err := svc.BlockSubject(context.Background(), "tenant-1", "actor-1", repo.request.ID); err != common.ErrConflict {
		t.Fatalf("terminal blocking request must not execute, got %v", err)
	}
	repo.request.Status = "in_progress"
	if err := svc.BlockSubject(context.Background(), "tenant-1", "actor-1", repo.request.ID); err != nil {
		t.Fatalf("in-progress blocking request should execute: %v", err)
	}
}

func TestExportRejectsNonExportRequest(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	repo := &fakePrivacyRepo{request: privacy.DataSubjectRequest{
		ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1",
		SubjectType: "customer", SubjectID: &subjectID, RequestType: "correction", Status: "in_progress",
	}}
	svc := newPrivacyService(repo)

	if _, err := svc.ExportSubjectData(context.Background(), "tenant-1", repo.request.ID); err != common.ErrConflict {
		t.Fatalf("non-export request must not execute export, got %v", err)
	}
}

func TestConsentLifecycle(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	repo := &fakePrivacyRepo{}
	svc := newPrivacyService(repo)
	id, err := svc.RecordConsent(context.Background(), "tenant-1", "req-1", ConsentCreateRequest{
		SubjectType:        "customer",
		SubjectID:          &subjectID,
		Purpose:            "marketing",
		ConsentTextVersion: "v1",
		Source:             "web",
	})
	if err != nil {
		t.Fatalf("RecordConsent returned error: %v", err)
	}
	if id == "" || repo.consentTenant != "tenant-1" {
		t.Fatalf("expected tenant-scoped consent, id=%q tenant=%q", id, repo.consentTenant)
	}
	if err := svc.RevokeConsent(context.Background(), "tenant-1", "22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatalf("RevokeConsent returned error: %v", err)
	}
	if !repo.revoked {
		t.Fatalf("expected consent revocation")
	}
}

func TestStatusFlowValidation(t *testing.T) {
	repo := &fakePrivacyRepo{request: privacy.DataSubjectRequest{ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1", Status: "open"}}
	svc := newPrivacyService(repo)
	validID := "22222222-2222-2222-2222-222222222222"
	for _, status := range []string{"in_progress", "rejected", "cancelled"} {
		repo.request.Status = "open"
		if err := svc.UpdateRequestStatus(context.Background(), "tenant-1", validID, UpdateStatusRequest{Status: status}); err != nil {
			t.Fatalf("status %q should be valid: %v", status, err)
		}
	}
	repo.request.Status = "open"
	if err := svc.UpdateRequestStatus(context.Background(), "tenant-1", validID, UpdateStatusRequest{Status: "open"}); err != common.ErrConflict {
		t.Fatalf("expected same-status update to be rejected, got %v", err)
	}
	if err := svc.UpdateRequestStatus(context.Background(), "tenant-1", validID, UpdateStatusRequest{Status: "in_review"}); err != common.ErrValidation {
		t.Fatalf("expected legacy status to be rejected, got %v", err)
	}
}

func TestStatusTerminalTransitionsAreRejected(t *testing.T) {
	repo := &fakePrivacyRepo{request: privacy.DataSubjectRequest{ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1", Status: "completed"}}
	svc := newPrivacyService(repo)
	for _, tc := range []struct {
		from string
		to   string
	}{
		{"completed", "in_progress"},
		{"completed", "completed"},
		{"rejected", "rejected"},
		{"cancelled", "cancelled"},
		{"cancelled", "open"},
	} {
		repo.request.Status = tc.from
		err := svc.UpdateRequestStatus(context.Background(), "tenant-1", repo.request.ID, UpdateStatusRequest{Status: tc.to})
		if err != common.ErrConflict {
			t.Fatalf("expected %s -> %s conflict, got %v", tc.from, tc.to, err)
		}
	}

	repo.request.Status = "in_progress"
	if err := svc.UpdateRequestStatus(context.Background(), "tenant-1", repo.request.ID, UpdateStatusRequest{Status: "completed"}); err != nil {
		t.Fatalf("expected in_progress -> completed to be valid: %v", err)
	}
}

func TestCrossTenantRequestAccessIsDeniedByRepositoryScope(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	repo := &fakePrivacyRepo{
		expectedTenant: "tenant-1",
		request:        privacy.DataSubjectRequest{ID: "22222222-2222-2222-2222-222222222222", TenantID: "tenant-1", SubjectType: "customer", SubjectID: &subjectID},
	}
	svc := newPrivacyService(repo)
	_, err := svc.ExportSubjectData(context.Background(), "tenant-2", repo.request.ID)
	if err != common.ErrNotFound {
		t.Fatalf("expected cross-tenant export to be denied, got %v", err)
	}
}

func newPrivacyService(repo Repository) *Service {
	return NewService(privacyFakeUOW{}, repo, nil)
}

type privacyFakeUOW struct{}

func (privacyFakeUOW) Begin(context.Context) (db.Tx, error) {
	return privacyFakeTx{}, nil
}

type privacyFakeTx struct{}

func (privacyFakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (privacyFakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (privacyFakeTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return privacyFakeRow{}
}
func (privacyFakeTx) Commit(context.Context) error   { return nil }
func (privacyFakeTx) Rollback(context.Context) error { return nil }

type privacyFakeRow struct{}

func (privacyFakeRow) Scan(...any) error { return nil }

type fakePrivacyRepo struct {
	createdTenant  string
	consentTenant  string
	exportedTenant string
	expectedTenant string
	subjectAllowed *bool
	anonymized     bool
	revoked        bool
	request        privacy.DataSubjectRequest
}

func (f *fakePrivacyRepo) SubjectBelongsToTenant(ctx context.Context, tenantID, subjectType, subjectID string) (bool, error) {
	if f.subjectAllowed != nil {
		return *f.subjectAllowed, nil
	}
	return true, nil
}

func (f *fakePrivacyRepo) CreateRequest(ctx context.Context, tenantID, actorUserID, requestID string, req CreateRequest) (string, error) {
	f.createdTenant = tenantID
	return "22222222-2222-2222-2222-222222222222", nil
}

func (f *fakePrivacyRepo) ListRequests(ctx context.Context, tenantID string, limit, offset int) ([]privacy.DataSubjectRequest, error) {
	return []privacy.DataSubjectRequest{{ID: "22222222-2222-2222-2222-222222222222", TenantID: tenantID}}, nil
}

func (f *fakePrivacyRepo) GetRequest(ctx context.Context, tenantID, id string) (privacy.DataSubjectRequest, error) {
	if f.expectedTenant != "" && tenantID != f.expectedTenant {
		return privacy.DataSubjectRequest{}, common.ErrNotFound
	}
	return f.request, nil
}

func (f *fakePrivacyRepo) GetRequestForUpdate(ctx context.Context, tx db.DBTX, tenantID, id string) (privacy.DataSubjectRequest, error) {
	return f.GetRequest(ctx, tenantID, id)
}

func (f *fakePrivacyRepo) UpdateRequestStatus(ctx context.Context, tenantID, id, status string, notes *string) error {
	f.request.Status = status
	return nil
}

func (f *fakePrivacyRepo) UpdateRequestStatusTx(ctx context.Context, tx db.DBTX, tenantID, id, status string, notes *string) error {
	f.request.Status = status
	return nil
}

func (f *fakePrivacyRepo) ExportSubjectData(ctx context.Context, tenantID, subjectType, subjectID string) (map[string]any, error) {
	f.exportedTenant = tenantID
	return map[string]any{"subject_type": subjectType, "id": subjectID}, nil
}

func (f *fakePrivacyRepo) AnonymizeSubject(ctx context.Context, tx db.DBTX, tenantID, subjectType, subjectID string) error {
	f.anonymized = true
	return nil
}

func (f *fakePrivacyRepo) BlockSubject(ctx context.Context, tx db.DBTX, tenantID, subjectType, subjectID string) error {
	return nil
}

func (f *fakePrivacyRepo) RecordConsent(ctx context.Context, tenantID, requestID string, req ConsentCreateRequest) (string, error) {
	f.consentTenant = tenantID
	return "22222222-2222-2222-2222-222222222222", nil
}

func (f *fakePrivacyRepo) ListConsents(ctx context.Context, tenantID string, limit, offset int) ([]privacy.ConsentRecord, error) {
	return []privacy.ConsentRecord{{ID: "22222222-2222-2222-2222-222222222222", TenantID: tenantID}}, nil
}

func (f *fakePrivacyRepo) RevokeConsent(ctx context.Context, tenantID, id string) error {
	f.revoked = true
	return nil
}
