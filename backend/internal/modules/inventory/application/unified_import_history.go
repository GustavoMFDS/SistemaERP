package application

import (
	"context"

	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

type UnifiedImportHistoryPage struct {
	Items   []inv.UnifiedImportReceipt `json:"items"`
	Limit   int                        `json:"limit"`
	Offset  int                        `json:"offset"`
	HasMore bool                       `json:"has_more"`
}

// Authorization is supplied from the server's freshly loaded RBAC context,
// never from the client. Do not reveal either stream when both flags are false.
func (s *ProductsService) ListUnifiedImportHistory(
	ctx context.Context, tenantID string, allowProducts, allowOpeningStock bool,
	limit, offset int, filter ImportHistoryFilter,
) (UnifiedImportHistoryPage, error) {
	if !allowProducts && !allowOpeningStock {
		return UnifiedImportHistoryPage{}, common.ErrForbidden
	}
	limit, offset, err := normalizedImportHistoryPage(limit, offset)
	if err != nil || ValidateImportHistoryFilter(filter) != nil {
		return UnifiedImportHistoryPage{}, common.ErrValidation
	}
	items, hasMore, err := s.repo.ListUnifiedImportHistory(
		ctx, tenantID, allowProducts, allowOpeningStock, limit, offset, filter.From, filter.To,
	)
	if err != nil {
		return UnifiedImportHistoryPage{}, err
	}
	return UnifiedImportHistoryPage{Items: items, Limit: limit, Offset: offset, HasMore: hasMore}, nil
}
