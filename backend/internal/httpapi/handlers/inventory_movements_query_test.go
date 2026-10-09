package handlers

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
)

func TestReadMovementsQueryFailsClosedOnMalformedFilters(t *testing.T) {
	tests := []string{
		"/api/v1/inventory/movements?product_id=not-a-uuid",
		"/api/v1/inventory/movements?limit=-1",
		"/api/v1/inventory/movements?limit=501",
		"/api/v1/inventory/movements?limit=abc",
		"/api/v1/inventory/movements?limit=1&limit=2",
		"/api/v1/inventory/movements?offset=-1",
		"/api/v1/inventory/movements?offset=5001",
		"/api/v1/inventory/movements?offset=abc",
		"/api/v1/inventory/movements?tenant_id=another-company",
		"/api/v1/inventory/movements?from=2026-10-01",
	}
	for _, url := range tests {
		_, _, _, err := readMovementsQuery(httptest.NewRequest("GET", url, nil))
		if !errors.Is(err, common.ErrValidation) {
			t.Errorf("unsafe query %q was not rejected: %v", url, err)
		}
	}
}

func TestReadMovementsQueryAcceptsBoundedPages(t *testing.T) {
	productID := "00000000-0000-4000-8000-000000000333"
	cases := []struct {
		url string
		product string
		limit, offset int
	}{
		{"/api/v1/inventory/movements", "", 100, 0},
		{"/api/v1/inventory/movements?limit=20&offset=40", "", 20, 40},
		{"/api/v1/inventory/movements?limit=500&offset=5000&product_id="+productID, productID, 500, 5000},
	}
	for _, tc := range cases {
		id, limit, offset, err := readMovementsQuery(httptest.NewRequest("GET", tc.url, nil))
		if err != nil || id != tc.product || limit != tc.limit || offset != tc.offset {
			t.Errorf("valid query %q mismatch: id=%q limit=%d offset=%d err=%v",
				tc.url, id, limit, offset, err)
		}
	}
}
