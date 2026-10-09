package application

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/modules/common"
)

const (
	maxImportHistoryPage = 50
	maxImportHistoryOffset = 5000
)

type ImportHistoryPage struct {
	Items []inv.ImportBatchEntry `json:"items"`
	Limit int `json:"limit"`
	Offset int `json:"offset"`
	HasMore bool `json:"has_more"`
}

func normalizedImportHistoryPage(limit, offset int) (int, int, error) {
	if offset < 0 || offset > maxImportHistoryOffset || limit < 0 || limit > maxImportHistoryPage {
		return 0, 0, common.ErrValidation
	}
	if limit == 0 {
		limit = 20
	}
	return limit, offset, nil
}

func (s *ProductsService) ListImportHistory(ctx context.Context, tenantID string, limit, offset int) (ImportHistoryPage, error) {
	limit, offset, err := normalizedImportHistoryPage(limit, offset)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	items, more, err := s.repo.ListProductImportHistory(ctx, tenantID, limit, offset)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	return ImportHistoryPage{Items: items, Limit: limit, Offset: offset, HasMore: more}, nil
}

func (s *InventoryService) ListOpeningStockHistory(ctx context.Context, tenantID string, limit, offset int) (ImportHistoryPage, error) {
	limit, offset, err := normalizedImportHistoryPage(limit, offset)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	items, more, err := s.inv.ListOpeningStockHistory(ctx, tenantID, limit, offset)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	return ImportHistoryPage{Items: items, Limit: limit, Offset: offset, HasMore: more}, nil
}
