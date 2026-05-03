package application

import (
	"context"
	"log/slog"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/go-playground/validator/v10"
)

type FinanceService struct {
	repo     FinanceRepository
	validate *validator.Validate
	logger   *slog.Logger
}

func NewFinanceService(repo FinanceRepository, v *validator.Validate, logger *slog.Logger) *FinanceService {
	return &FinanceService{repo: repo, validate: v, logger: logger}
}

func (s *FinanceService) Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]platform.Money, error) {
	return s.repo.Dashboard(ctx, tenantID, from, to)
}

func (s *FinanceService) ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error) {
	return s.repo.ListLedger(ctx, tenantID, limit, offset)
}
