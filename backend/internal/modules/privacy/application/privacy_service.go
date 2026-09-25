package application

import (
	"context"
	"net/mail"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/google/uuid"
)

type Service struct {
	repo Repository
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

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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
	return s.repo.CreateRequest(ctx, tenantID, actorUserID, requestID, req)
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

func (s *Service) UpdateRequestStatus(ctx context.Context, tenantID, id string, req UpdateStatusRequest) error {
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if !isUUID(id) || !validStatus(req.Status) {
		return common.ErrValidation
	}
	current, err := s.repo.GetRequest(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if !validStatusTransition(current.Status, req.Status) {
		return common.ErrConflict
	}
	return s.repo.UpdateRequestStatus(ctx, tenantID, id, req.Status, req.Notes)
}

func (s *Service) ExportSubjectData(ctx context.Context, tenantID, requestID string) (map[string]any, error) {
	req, err := s.repo.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return nil, common.ErrValidation
	}
	return s.repo.ExportSubjectData(ctx, tenantID, req.SubjectType, *req.SubjectID)
}

func (s *Service) AnonymizeSubject(ctx context.Context, tenantID, requestID string) error {
	req, err := s.repo.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return common.ErrValidation
	}
	return s.repo.AnonymizeSubject(ctx, tenantID, req.SubjectType, *req.SubjectID)
}

func (s *Service) BlockSubject(ctx context.Context, tenantID, requestID string) error {
	req, err := s.repo.GetRequest(ctx, tenantID, requestID)
	if err != nil {
		return err
	}
	if req.SubjectID == nil || strings.TrimSpace(*req.SubjectID) == "" {
		return common.ErrValidation
	}
	return s.repo.BlockSubject(ctx, tenantID, req.SubjectType, *req.SubjectID)
}

func (s *Service) RecordConsent(ctx context.Context, tenantID, requestID string, req ConsentCreateRequest) (string, error) {
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
	return s.repo.RecordConsent(ctx, tenantID, requestID, req)
}

func (s *Service) ListConsents(ctx context.Context, tenantID string, limit, offset int) (any, error) {
	limit, offset = normalizePagination(limit, offset)
	return s.repo.ListConsents(ctx, tenantID, limit, offset)
}

func (s *Service) RevokeConsent(ctx context.Context, tenantID, id string) error {
	if !isUUID(id) {
		return common.ErrValidation
	}
	return s.repo.RevokeConsent(ctx, tenantID, id)
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
