package application

import (
	"context"
	"errors"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

const (
	maxImportHistoryPage   = 50
	maxImportHistoryOffset = 5000
	maxHistoryExportRows   = 1000
	dateLayout             = "2006-01-02"
)

var ErrImportHistoryExportTooLarge = errors.New("too many import receipts to export")

// ImportHistoryFilter is parsed as calendar dates in the store's Brazilian
// timezone by PostgreSQL. Empty bounds mean no restriction.
type ImportHistoryFilter struct {
	From string
	To   string
}

func ValidateImportHistoryFilter(filter ImportHistoryFilter) error {
	var fromDate, toDate time.Time
	for _, entry := range []struct {
		value string
		out   *time.Time
	}{{filter.From, &fromDate}, {filter.To, &toDate}} {
		if entry.value == "" {
			continue
		}
		if len(entry.value) != len(dateLayout) {
			return common.ErrValidation
		}
		date, err := time.Parse(dateLayout, entry.value)
		if err != nil || date.Format(dateLayout) != entry.value {
			return common.ErrValidation
		}
		*entry.out = date
	}
	if !fromDate.IsZero() && !toDate.IsZero() {
		if fromDate.After(toDate) || toDate.Sub(fromDate) > 365*24*time.Hour {
			return common.ErrValidation
		}
	}
	return nil
}

type ImportHistoryPage struct {
	Items   []inv.ImportBatchEntry `json:"items"`
	Limit   int                    `json:"limit"`
	Offset  int                    `json:"offset"`
	HasMore bool                   `json:"has_more"`
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

func (s *ProductsService) ListImportHistory(
	ctx context.Context, tenantID string, limit, offset int, filter ImportHistoryFilter,
) (ImportHistoryPage, error) {
	limit, offset, err := normalizedImportHistoryPage(limit, offset)
	if err != nil || ValidateImportHistoryFilter(filter) != nil {
		return ImportHistoryPage{}, common.ErrValidation
	}
	items, more, err := s.repo.ListProductImportHistory(ctx, tenantID, limit, offset, filter.From, filter.To)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	return ImportHistoryPage{Items: items, Limit: limit, Offset: offset, HasMore: more}, nil
}

func (s *InventoryService) ListOpeningStockHistory(
	ctx context.Context, tenantID string, limit, offset int, filter ImportHistoryFilter,
) (ImportHistoryPage, error) {
	limit, offset, err := normalizedImportHistoryPage(limit, offset)
	if err != nil || ValidateImportHistoryFilter(filter) != nil {
		return ImportHistoryPage{}, common.ErrValidation
	}
	items, more, err := s.inv.ListOpeningStockHistory(ctx, tenantID, limit, offset, filter.From, filter.To)
	if err != nil {
		return ImportHistoryPage{}, err
	}
	return ImportHistoryPage{Items: items, Limit: limit, Offset: offset, HasMore: more}, nil
}

// Exports are read-only and bounded. Nothing is sent if there are more than
// 1,000 matching receipts: users must narrow the date range first.
func (s *ProductsService) ExportImportHistory(
	ctx context.Context, tenantID string, filter ImportHistoryFilter,
) ([]inv.ImportBatchEntry, error) {
	if err := ValidateImportHistoryFilter(filter); err != nil {
		return nil, err
	}
	items, more, err := s.repo.ListProductImportHistory(ctx, tenantID, maxHistoryExportRows, 0, filter.From, filter.To)
	if err != nil {
		return nil, err
	}
	if more {
		return nil, ErrImportHistoryExportTooLarge
	}
	return items, nil
}

func (s *InventoryService) ExportOpeningStockHistory(
	ctx context.Context, tenantID string, filter ImportHistoryFilter,
) ([]inv.ImportBatchEntry, error) {
	if err := ValidateImportHistoryFilter(filter); err != nil {
		return nil, err
	}
	items, more, err := s.inv.ListOpeningStockHistory(ctx, tenantID, maxHistoryExportRows, 0, filter.From, filter.To)
	if err != nil {
		return nil, err
	}
	if more {
		return nil, ErrImportHistoryExportTooLarge
	}
	return items, nil
}
