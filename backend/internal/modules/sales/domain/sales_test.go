package domain_test

import (
	"testing"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
)

func TestSaleItem_CalcularSubtotal_OK(t *testing.T) {
	it := sales.SaleItem{Qty: 2, UnitPrice: 10, DiscountValue: 1}
	gross, net, err := it.CalcularSubtotal()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gross != 20 {
		t.Fatalf("gross want 20, got %v", gross)
	}
	if net != 19 {
		t.Fatalf("net want 19, got %v", net)
	}
}

func TestSaleItem_CalcularSubtotal_InvalidItem(t *testing.T) {
	cases := []sales.SaleItem{
		{Qty: 0, UnitPrice: 10, DiscountValue: 0},
		{Qty: 1, UnitPrice: 0, DiscountValue: 0},
		{Qty: 1, UnitPrice: 10, DiscountValue: -1},
	}
	for _, c := range cases {
		_, _, err := c.CalcularSubtotal()
		if err != sales.ErrInvalidItem {
			t.Fatalf("want ErrInvalidItem, got %v", err)
		}
	}
}

func TestSale_CalcularTotal_ComputesTotals(t *testing.T) {
	s := sales.NewFinalizedSale("cs", nil, "u", 2) // sale-level discount = 2
	items := []sales.SaleItem{
		{ProductID: "p1", Qty: 2, UnitPrice: 10, DiscountValue: 1, CostUnit: 6},
		{ProductID: "p2", Qty: 1, UnitPrice: 5, DiscountValue: 0, CostUnit: 1},
	}
	computed, err := s.CalcularTotal(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(computed) != 2 {
		t.Fatalf("want 2 items, got %d", len(computed))
	}
	// subtotal = 2*10 + 1*5 = 25
	if s.Subtotal != 25 {
		t.Fatalf("subtotal want 25, got %v", s.Subtotal)
	}
	// total discount = item discount (1) + sale discount (2) = 3
	if s.DiscountValue != 3 {
		t.Fatalf("discount want 3, got %v", s.DiscountValue)
	}
	// total = 25 - 3 = 22
	if s.Total != 22 {
		t.Fatalf("total want 22, got %v", s.Total)
	}
}

func TestSale_ValidarPagamentos_OK(t *testing.T) {
	s := sales.Sale{Total: 10}
	err := s.ValidarPagamentos([]sales.Payment{{Method: "cash", Amount: 3}, {Method: "pix", Amount: 7}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSale_ValidarPagamentos_Mismatch(t *testing.T) {
	s := sales.Sale{Total: 10}
	err := s.ValidarPagamentos([]sales.Payment{{Method: "cash", Amount: 9.98}})
	if err != sales.ErrPaymentsMismatch {
		t.Fatalf("want ErrPaymentsMismatch, got %v", err)
	}
}

func TestSale_PodeCancelar(t *testing.T) {
	if err := (sales.Sale{Status: "cancelled"}).PodeCancelar(); err != sales.ErrSaleAlreadyCancelled {
		t.Fatalf("want ErrSaleAlreadyCancelled, got %v", err)
	}
	if err := (sales.Sale{Status: "draft"}).PodeCancelar(); err != sales.ErrSaleNotFinalized {
		t.Fatalf("want ErrSaleNotFinalized, got %v", err)
	}
	if err := (sales.Sale{Status: "finalized"}).PodeCancelar(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
