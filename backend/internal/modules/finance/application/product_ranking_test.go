package application

import (
	"errors"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
)

func TestProductRankingRangeValidation(t *testing.T) {
	svc := &FinanceService{}
	cases := []struct {
		from, to string
		limit    int
	}{
		{"", "2026-10-08", 10},
		{"2026-10-08", "", 10},
		{"2026-02-30", "2026-10-08", 10},
		{"2026-10-09", "2026-10-08", 10},
		{"2025-01-01", "2026-10-08", 10},
		{"2026-10-08", "2026-10-08", 0},
		{"2026-10-08", "2026-10-08", 51},
		{"2026-10-08 OR true", "2026-10-08", 10},
	}
	for _, tc := range cases {
		_, err := svc.ProductRanking(t.Context(), "tenant", tc.from, tc.to, tc.limit)
		if !errors.Is(err, common.ErrValidation) {
			t.Errorf("invalid ranking filter %+v should fail validation, got %v", tc, err)
		}
	}
}
