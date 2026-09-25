package application

import (
	"context"
	"net/mail"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/google/uuid"
)

type Service struct {
	uow   db.UnitOfWork
	repo  Repository
	audit *audit.Service
}

type CreateRequest struct {
	SubjectType    string  `json:"subject_type"`
	SubjectID      *string `json:"subject_id"`
	RequesterEmail *string `json:"requester_email"`
	RequestType    string  `json:"request_type"`
	Notes          *string `json:"notes"`
}

type UpdateStatusRequest struct {
	Status string  `json:"status"`
	Notes  *string `json:"notes"`
}

type ConsentCreateRequest struct {
	SubjectType        string  `json:"subject_type"`
	SubjectID          *string `json:"subject_id"`
	Purpose            string  `json:"purpose"`
	ConsentTextVersion string  `json:"consent_text_version"`
	Source             string  `json:"source"`
}

func NewService(uow db.UnitOfWork, repo Repository, auditSvc *audit.Service) *Service {
	return &Service{uow: uow, repo: repo, audit: auditSvc}
}

func (s *Service) CreateRequest(ctx context.Context, tenantID, actorUserID, requestID string, req CreateRequest) (string, error) {
	req.normalize()
	if err := validateCreateRequest(req); err != nil {
		return "", err
	}
	if req.SubjectID != nil {
		ok, err := s.repo.SubjectBelongsToTenant(ctx, tenantID, req.SubjectType, *req.SubjectID)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", common.ErrNotFound
		}
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.repo.CreateRequest(ctx, tx, tenantID, actorUserID, requestID, req)
	if err != nil {
		return "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.request.create",
		ResourceType: "data_subject_request", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{"subject_type": req.SubjectType, "request_type": req.RequestType},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) ListRequests(ctx context.Context, tenantID string, limit, offset int) (any, error) {
	limit, offset = normalizePagination(limit, offset)
	return s.repo.ListRequests(ctx, tenantID, limit, offset)
}

func (s *Service) GetRequest(ctx context.Context, tenantID, id string) (any, error) {
	if !isUUID(id) {
		return nil, common.ErrValidation
	}
	return s.repo.GetRequest(ctx, tenantID, id)
}

func (s *Service) UpdateRequestStatus(ctx context.Context, tenantID, actorUserID, id string, req UpdateStatusRequest) error {
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if !isUUID(id) || !validStatus(req.Status) {
		return common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.repo.GetRequestForUpdate(ctx, tx, tenantID, id)
	if err != nil {
		return err
	}
	if !validStatusTransition(current.Status, req.Status) {
		return common.ErrConflict
	}
	if err := s.repo.UpdateRequestStatus(ctx, tx, tenantID, id, req.Status, req.Notes); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.request.update",
		ResourceType: "data_subject_request", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{"status": req.Status},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ExportSubjectData(ctx context.Context, tenantID, actorUserID, requestID string) (map[string]any, error) {
	if !isUUID(requestID) {
		return nil, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := s.repo.GetRequestForUpdate(ctx, tx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return nil, common.ErrValidation
	}
	if req.RequestType != "export" || req.Status != "in_progress" {
		return nil, common.ErrConflict
	}

	data, err := s.repo.ExportSubjectData(ctx, tenantID, req.SubjectType, *req.SubjectID)
	if err != nil {
		return nil, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.subject.export",
		ResourceType: "data_subject_request", ResourceID: requestID, Outcome: "success",
		Metadata: map[string]any{"subject_type": req.SubjectType},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) AnonymizeSubject(ctx context.Context, tenantID, actorUserID, requestID string) error {
	if !isUUID(requestID) {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := s.repo.GetRequestForUpdate(ctx, tx, tenantID, requestID)
	if err != nil {
		return err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return common.ErrValidation
	}
	if req.Status != "in_progress" {
		return common.ErrConflict
	}
	if req.RequestType != "anonymization" && req.RequestType != "deletion" {
		return common.ErrConflict
	}
	if err := s.repo.AnonymizeSubject(ctx, tx, tenantID, req.SubjectType, *req.SubjectID); err != nil {
		return err
	}
	if err := s.repo.UpdateRequestStatus(ctx, tx, tenantID, requestID, "completed", nil); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.subject.anonymize",
		ResourceType: "data_subject_request", ResourceID: requestID, Outcome: "success",
		Metadata: map[string]any{"subject_type": req.SubjectType, "request_type": req.RequestType},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) BlockSubject(ctx context.Context, tenantID, actorUserID, requestID string) error {
	if !isUUID(requestID) {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	req, err := s.repo.GetRequestForUpdate(ctx, tx, tenantID, requestID)
	if err != nil {
		return err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return common.ErrValidation
	}
	if req.Status != "in_progress" || req.RequestType != "blocking" {
		return common.ErrConflict
	}
	if err := s.repo.BlockSubject(ctx, tx, tenantID, req.SubjectType, *req.SubjectID); err != nil {
		return err
	}
	if err := s.repo.UpdateRequestStatus(ctx, tx, tenantID, requestID, "completed", nil); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.subject.block",
		ResourceType: "data_subject_request", ResourceID: requestID, Outcome: "success",
		Metadata: map[string]any{"subject_type": req.SubjectType, "request_type": req.RequestType},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RecordConsent(ctx context.Context, tenantID, actorUserID, requestID string, req ConsentCreateRequest) (string, error) {
	req.normalize()
	if err := validateConsent(req); err != nil {
		return "", err
	}
	ok, err := s.repo.SubjectBelongsToTenant(ctx, tenantID, req.SubjectType, *req.SubjectID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", common.ErrNotFound
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.repo.RecordConsent(ctx, tx, tenantID, requestID, req)
	if err != nil {
		return "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.consent.create",
		ResourceType: "consent_record", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{"purpose": req.Purpose, "subject_type": req.SubjectType},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) ListConsents(ctx context.Context, tenantID string, limit, offset int) (any, error) {
	limit, offset = normalizePagination(limit, offset)
	return s.repo.ListConsents(ctx, tenantID, limit, offset)
}

func (s *Service) RevokeConsent(ctx context.Context, tenantID, actorUserID, id string) error {
	if !isUUID(id) {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.RevokeConsent(ctx, tx, tenantID, id); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "privacy.consent.revoke",
		ResourceType: "consent_record", ResourceID: id, Outcome: "success",
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *CreateRequest) normalize() {
	r.SubjectType = strings.ToLower(strings.TrimSpace(r.SubjectType))
	r.RequestType = strings.ToLower(strings.TrimSpace(r.RequestType))
	trimPtr(r.SubjectID)
	trimPtr(r.RequesterEmail)
	trimPtr(r.Notes)
}

func (r *ConsentCreateRequest) normalize() {
	r.SubjectType = strings.ToLower(strings.TrimSpace(r.SubjectType))
	r.Purpose = strings.TrimSpace(r.Purpose)
	r.ConsentTextVersion = strings.TrimSpace(r.ConsentTextVersion)
	r.Source = strings.TrimSpace(r.Source)
	trimPtr(r.SubjectID)
}

func validateCreateRequest(req CreateRequest) error {
	if !validSubjectType(req.SubjectType) || !validRequestType(req.RequestType) {
		return common.ErrValidation
	}
	if req.RequestType == "blocking" && req.SubjectType != "user" {
		return common.ErrValidation
	}
	if req.SubjectID != nil && !isUUID(*req.SubjectID) {
		return common.ErrValidation
	}
	if req.RequesterEmail != nil && *req.RequesterEmail != "" {
		if len(*req.RequesterEmail) > 254 {
			return common.ErrValidation
		}
		if _, err := mail.ParseAddress(*req.RequesterEmail); err != nil {
			return common.ErrValidation
		}
	}
	if req.SubjectID == nil && (req.RequesterEmail == nil || *req.RequesterEmail == "") {
		return common.ErrValidation
	}
	return nil
}

func validateConsent(req ConsentCreateRequest) error {
	if !validSubjectType(req.SubjectType) || req.SubjectID == nil || !isUUID(*req.SubjectID) {
		return common.ErrValidation
	}
	if len(req.Purpose) < 3 || len(req.Purpose) > 120 {
		return common.ErrValidation
	}
	if len(req.ConsentTextVersion) < 1 || len(req.ConsentTextVersion) > 64 {
		return common.ErrValidation
	}
	if len(req.Source) < 2 || len(req.Source) > 80 {
		return common.ErrValidation
	}
	return nil
}

func validSubjectType(v string) bool {
	switch v {
	case "customer", "user":
		return true
	default:
		return false
	}
}

func validRequestType(v string) bool {
	switch v {
	case "export", "correction", "anonymization", "deletion", "blocking":
		return true
	default:
		return false
	}
}

func validStatus(v string) bool {
	switch v {
	case "open", "in_progress", "completed", "rejected", "cancelled":
		return true
	default:
		return false
	}
}

func validStatusTransition(from, to string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	switch from {
	case "open":
		return to == "in_progress" || to == "cancelled" || to == "rejected"
	case "in_progress":
		return to == "completed" || to == "rejected" || to == "cancelled"
	default:
		return false
	}
}

func isUUID(v string) bool {
	_, err := uuid.Parse(strings.TrimSpace(v))
	return err == nil
}

func trimPtr(v *string) {
	if v != nil {
		*v = strings.TrimSpace(*v)
	}
}

func normalizePagination(limit, offset int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
