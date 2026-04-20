package service

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/repo"
	"github.com/go-playground/validator/v10"
)

type FinanceService struct {
	fin      *repo.FinanceRepo
	validate *validator.Validate
	logger   *slog.Logger
}

func NewFinanceService(fin *repo.FinanceRepo, v *validator.Validate, logger *slog.Logger) *FinanceService {
	return &FinanceService{fin: fin, validate: v, logger: logger}
}

func (s *FinanceService) Dashboard(ctx context.Context, from, to string) (map[string]float64, error) {
	return s.fin.Dashboard(ctx, from, to)
}

func (s *FinanceService) ListLedger(ctx context.Context, limit, offset int) ([]repo.LedgerEntry, int, error) {
	return s.fin.ListLedger(ctx, limit, offset)
}
