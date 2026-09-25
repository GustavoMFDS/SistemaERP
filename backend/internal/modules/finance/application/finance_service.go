package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/sistemaemgo/internal/config"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/go-playground/validator/v10"
)

type FinanceService struct {
	repo             FinanceRepository
	businessTimezone string
	location         *time.Location
	validate         *validator.Validate
	logger           *slog.Logger
}

func NewFinanceService(cfg config.Config, repo FinanceRepository, v *validator.Validate, logger *slog.Logger) *FinanceService {
	timezone := cfg.BusinessTimezone
	if timezone == "" {
		timezone = "America/Sao_Paulo"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	return &FinanceService{
		repo: repo, businessTimezone: timezone, location: location, validate: v, logger: logger,
	}
}

func (s *FinanceService) Today() string {
	return time.Now().In(s.location).Format("2006-01-02")
}

func (s *FinanceService) Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]platform.Money, error) {
	return s.repo.Dashboard(ctx, tenantID, from, to, s.businessTimezone)
}

func (s *FinanceService) ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error) {
	return s.repo.ListLedger(ctx, tenantID, limit, offset)
}
