package application

import (
	"errors"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
)

func TestImportHistoryDateFilterValidation(t *testing.T) {
	valid := []ImportHistoryFilter{
		{}, {From: "2026-10-08"}, {To: "2026-10-08"},
		{From: "2026-10-08", To: "2026-10-08"},
		{From: "2026-01-01", To: "2026-12-31"},
	}
	for _, filter := range valid {
		if err := ValidateImportHistoryFilter(filter); err != nil {
			t.Errorf("valid filter %+v rejected: %v", filter, err)
		}
	}
	invalid := []ImportHistoryFilter{
		{From: "2026-2-01"},
		{From: "2026-02-30"},
		{From: "2026-10-08T00:00:00Z"},
		{From: "2026-10-08' OR true --"},
		{From: "2026-10-09", To: "2026-10-08"},
		{From: "2024-01-01", To: "2026-10-08"},
		{To: "2026-13-01"},
		{To: " 2026-10-08"},
	}
	for _, filter := range invalid {
		if err := ValidateImportHistoryFilter(filter); !errors.Is(err, common.ErrValidation) {
			t.Errorf("invalid filter %+v must be rejected: %v", filter, err)
		}
	}
}

func TestImportHistoryPaginationValidation(t *testing.T) {
	for _, item := range []struct{ limit, offset int }{
		{1, 0}, {50, 5000}, {0, 0},
	} {
		if _, _, err := normalizedImportHistoryPage(item.limit, item.offset); err != nil {
			t.Fatalf("valid page %+v: %v", item, err)
		}
	}
	for _, item := range []struct{ limit, offset int }{
		{51, 0}, {-1, 0}, {20, -1}, {20, 5001},
	} {
		if _, _, err := normalizedImportHistoryPage(item.limit, item.offset); !errors.Is(err, common.ErrValidation) {
			t.Fatalf("invalid page %+v should be rejected: %v", item, err)
		}
	}
}
