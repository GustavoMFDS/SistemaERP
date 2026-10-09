package application

import (
	"context"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
)

func (s *FinanceService) ProductRanking(
	ctx context.Context, tenantID, from, to string, limit int,
) ([]fin.ProductRankingRow, error) {
	if len(from) != 10 || len(to) != 10 || limit < 1 || limit > 50 {
		return nil, common.ErrValidation
	}
	first, err := time.Parse("2006-01-02", from)
	if err != nil || first.Format("2006-01-02") != from {
		return nil, common.ErrValidation
	}
	last, err := time.Parse("2006-01-02", to)
	if err != nil || last.Format("2006-01-02") != to ||
		last.Before(first) || last.Sub(first) > 366*24*time.Hour {
		return nil, common.ErrValidation
	}
	return s.repo.ProductRanking(ctx, tenantID, from, to, limit)
}
