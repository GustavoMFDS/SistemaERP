package application

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type CashService struct {
	uow      db.UnitOfWork
	cash     CashRepository
	validate *validator.Validate
	logger   *slog.Logger
}

type CashOpenRequest struct {
	OpeningAmount platform.Money `json:"opening_amount" validate:"min=0"`
	Notes         *string        `json:"notes"`
}

type CashCloseRequest struct {
	ClosingAmount platform.Money `json:"closing_amount" validate:"min=0"`
	Notes         *string        `json:"notes"`
}

func NewCashService(uow db.UnitOfWork, cash CashRepository, v *validator.Validate, logger *slog.Logger) *CashService {
	return &CashService{uow: uow, cash: cash, validate: v, logger: logger}
}

func (s *CashService) OpenSession(ctx context.Context, tenantID string, userID string, req CashOpenRequest) (string, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	registerID, err := s.cash.EnsureDefaultRegister(ctx, tenantID)
	if err != nil {
		return "", err
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.cash.OpenSession(ctx, tx, tenantID, registerID, userID, req.OpeningAmount, req.Notes)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *CashService) CloseSession(ctx context.Context, tenantID string, userID, sessionID string, req CashCloseRequest) (sales.CashCloseResult, error) {
	if err := s.validate.Struct(req); err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result, err := s.cash.CloseSession(ctx, tx, tenantID, sessionID, userID, req.ClosingAmount, req.Notes)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sales.CashCloseResult{}, err
	}
	return result, nil
}
