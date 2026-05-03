package domain_test

import (
	"testing"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func TestSaleItem_CalcularSubtotal_OK(t *testing.T) {
	it := sales.SaleItem{Qty: platform.NewQuantityMilli(2_000), UnitPrice: platform.NewMoneyCents(1000), DiscountValue: platform.NewMoneyCents(100)}
	gross, net, err := it.CalcularSubtotal()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gross.Cents() != 2000 {
		t.Fatalf("gross want 2000 cents, got %d", gross.Cents())
	}
	if net.Cents() != 1900 {
		t.Fatalf("net want 1900 cents, got %d", net.Cents())
	}
}

func TestSaleItem_CalcularSubtotal_InvalidItem(t *testing.T) {
	cases := []sales.SaleItem{
		{Qty: 0, UnitPrice: platform.NewMoneyCents(1000), DiscountValue: 0},
		{Qty: platform.NewQuantityMilli(1_000), UnitPrice: 0, DiscountValue: 0},
		{Qty: platform.NewQuantityMilli(1_000), UnitPrice: platform.NewMoneyCents(1000), DiscountValue: platform.NewMoneyCents(-100)},
	}
	for _, c := range cases {
		_, _, err := c.CalcularSubtotal()
		if err != sales.ErrInvalidItem {
			t.Fatalf("want ErrInvalidItem, got %v", err)
		}
	}
}

func TestSale_CalcularTotal_ComputesTotals(t *testing.T) {
	s := sales.NewFinalizedSale("cs", nil, "u", platform.NewMoneyCents(200))
	items := []sales.SaleItem{
		{ProductID: "p1", Qty: platform.NewQuantityMilli(2_000), UnitPrice: platform.NewMoneyCents(1000), DiscountValue: platform.NewMoneyCents(100), CostUnit: platform.NewMoneyCents(600)},
		{ProductID: "p2", Qty: platform.NewQuantityMilli(1_000), UnitPrice: platform.NewMoneyCents(500), DiscountValue: 0, CostUnit: platform.NewMoneyCents(100)},
	}
	computed, err := s.CalcularTotal(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(computed) != 2 {
		t.Fatalf("want 2 items, got %d", len(computed))
	}
	if s.Subtotal.Cents() != 2500 {
		t.Fatalf("subtotal want 2500 cents, got %d", s.Subtotal.Cents())
	}
	if s.DiscountValue.Cents() != 300 {
		t.Fatalf("discount want 300 cents, got %d", s.DiscountValue.Cents())
	}
	if s.Total.Cents() != 2200 {
		t.Fatalf("total want 2200 cents, got %d", s.Total.Cents())
	}
}

func TestSale_ValidarPagamentos_OK(t *testing.T) {
	s := sales.Sale{Total: platform.NewMoneyCents(1000)}
	err := s.ValidarPagamentos([]sales.Payment{{Method: "cash", Amount: platform.NewMoneyCents(300)}, {Method: "pix", Amount: platform.NewMoneyCents(700)}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSale_ValidarPagamentos_Mismatch(t *testing.T) {
	s := sales.Sale{Total: platform.NewMoneyCents(1000)}
	err := s.ValidarPagamentos([]sales.Payment{{Method: "cash", Amount: platform.NewMoneyCents(998)}})
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
