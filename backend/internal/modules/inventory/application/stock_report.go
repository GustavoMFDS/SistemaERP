package application

import (
	"context"
	"errors"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

const MaxStockReportRows = 5000

var ErrStockReportTooLarge = errors.New("stock report exceeds 5000 rows")

// StockReport is a bounded read-only snapshot with no monetary information.
func (s *InventoryService) StockReport(ctx context.Context, tenantID string) ([]inv.StockReportRow, error) {
	items, err := s.inv.StockReport(ctx, tenantID, MaxStockReportRows)
	if err != nil {
		return nil, err
	}
	if len(items) > MaxStockReportRows {
		return nil, ErrStockReportTooLarge
	}
	return items, nil
}
