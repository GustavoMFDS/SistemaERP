package domain_test

import (
	"testing"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func TestProduct_PodeVender(t *testing.T) {
	p := inv.Product{Active: true, PriceCash: platform.NewMoneyCents(1000)}
	if err := p.PodeVender(platform.NewQuantityMilli(1_000)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (inv.Product{Active: false, PriceCash: platform.NewMoneyCents(1000)}).PodeVender(platform.NewQuantityMilli(1_000)); err != inv.ErrProductInactive {
		t.Fatalf("want ErrProductInactive, got %v", err)
	}
	if err := (inv.Product{Active: true, PriceCash: platform.NewMoneyCents(1000)}).PodeVender(0); err != inv.ErrInvalidQuantity {
		t.Fatalf("want ErrInvalidQuantity, got %v", err)
	}
	if err := (inv.Product{Active: true, PriceCash: 0}).PodeVender(platform.NewQuantityMilli(1_000)); err != inv.ErrInvalidPrice {
		t.Fatalf("want ErrInvalidPrice, got %v", err)
	}
}

func TestInventoryBalance_Baixar(t *testing.T) {
	b := inv.InventoryBalance{ProductID: "p1", QtyOnHand: platform.NewQuantityMilli(10_000)}
	after, err := b.Baixar(platform.NewQuantityMilli(3_000), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after.QtyOnHand.Milli() != 7_000 {
		t.Fatalf("want 7000, got %d", after.QtyOnHand.Milli())
	}

	_, err = b.Baixar(platform.NewQuantityMilli(20_000), false)
	if err != inv.ErrInsufficientStock {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}

	after2, err := b.Baixar(platform.NewQuantityMilli(20_000), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after2.QtyOnHand.Milli() != -10_000 {
		t.Fatalf("want -10000, got %d", after2.QtyOnHand.Milli())
	}
}

func TestInventoryBalance_ApplyDeltaValidation(t *testing.T) {
	b := inv.InventoryBalance{ProductID: "p1", QtyOnHand: platform.NewQuantityMilli(10_000)}
	_, err := b.AplicarDelta(0, true)
	if err != inv.ErrInvalidDelta {
		t.Fatalf("want ErrInvalidDelta, got %v", err)
	}
}

func TestMovementType_NormalizeDelta(t *testing.T) {
	if d, err := inv.MovementSale.NormalizeDelta(platform.NewQuantityMilli(2_000)); err != nil || d.Milli() != -2_000 {
		t.Fatalf("want -2000 nil, got %v %v", d, err)
	}
	if d, err := inv.MovementPurchase.NormalizeDelta(platform.NewQuantityMilli(-2_000)); err != nil || d.Milli() != 2_000 {
		t.Fatalf("want 2000 nil, got %v %v", d, err)
	}
	if _, err := inv.MovementType("bad").NormalizeDelta(platform.NewQuantityMilli(1_000)); err != inv.ErrInvalidMovementType {
		t.Fatalf("want ErrInvalidMovementType, got %v", err)
	}
}

func TestInventoryBalance_RejectsQuantityRangeOverflow(t *testing.T) {
	b := inv.InventoryBalance{
		ProductID: "p1",
		QtyOnHand: platform.NewQuantityMilli(99_999_999_999_999),
	}
	if _, err := b.Creditar(platform.NewQuantityMilli(1)); err != inv.ErrInvalidQuantity {
		t.Fatalf("want ErrInvalidQuantity for quantity overflow, got %v", err)
	}

	min := inv.InventoryBalance{
		ProductID: "p1",
		QtyOnHand: platform.NewQuantityMilli(-99_999_999_999_999),
	}
	if _, err := min.AplicarDelta(platform.NewQuantityMilli(-1), true); err != inv.ErrInvalidQuantity {
		t.Fatalf("want ErrInvalidQuantity for negative quantity overflow, got %v", err)
	}
}
