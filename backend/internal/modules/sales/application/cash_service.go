package application

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CashService struct {
	cash     CashRepository
	validate *validator.Validate
	logger   *slog.Logger
}

type CashOpenRequest struct {
	OpeningAmount float64 `json:"opening_amount" validate:"min=0"`
	Notes         *string `json:"notes"`
}

type CashCloseRequest struct {
	ClosingAmount float64 `json:"closing_amount" validate:"min=0"`
	Notes         *string `json:"notes"`
}

func NewCashService(cash CashRepository, v *validator.Validate, logger *slog.Logger) *CashService {
	return &CashService{cash: cash, validate: v, logger: logger}
}

func (s *CashService) OpenSession(ctx context.Context, pool *pgxpool.Pool, userID string, req CashOpenRequest) (string, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	registerID, err := s.cash.EnsureDefaultRegister(ctx)
	if err != nil {
		return "", err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.cash.OpenSession(ctx, tx, registerID, userID, req.OpeningAmount, req.Notes)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *CashService) CloseSession(ctx context.Context, pool *pgxpool.Pool, userID, sessionID string, req CashCloseRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = s.cash.CloseSession(ctx, tx, sessionID, userID, req.ClosingAmount, req.Notes)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
