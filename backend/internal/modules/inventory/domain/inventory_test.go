package domain_test

import (
	"testing"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

func TestProduct_PodeVender(t *testing.T) {
	p := inv.Product{Active: true, PriceCash: 10}
	if err := p.PodeVender(1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := (inv.Product{Active: false, PriceCash: 10}).PodeVender(1); err != inv.ErrProductInactive {
		t.Fatalf("want ErrProductInactive, got %v", err)
	}
	if err := (inv.Product{Active: true, PriceCash: 10}).PodeVender(0); err != inv.ErrInvalidQuantity {
		t.Fatalf("want ErrInvalidQuantity, got %v", err)
	}
	if err := (inv.Product{Active: true, PriceCash: 0}).PodeVender(1); err != inv.ErrInvalidPrice {
		t.Fatalf("want ErrInvalidPrice, got %v", err)
	}
}

func TestInventoryBalance_Baixar(t *testing.T) {
	b := inv.InventoryBalance{ProductID: "p1", QtyOnHand: 10}
	after, err := b.Baixar(3, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after.QtyOnHand != 7 {
		t.Fatalf("want 7, got %v", after.QtyOnHand)
	}

	_, err = b.Baixar(20, false)
	if err != inv.ErrInsufficientStock {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}

	after2, err := b.Baixar(20, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after2.QtyOnHand != -10 {
		t.Fatalf("want -10, got %v", after2.QtyOnHand)
	}
}

func TestInventoryBalance_ApplyDeltaValidation(t *testing.T) {
	b := inv.InventoryBalance{ProductID: "p1", QtyOnHand: 10}
	_, err := b.AplicarDelta(0, true)
	if err != inv.ErrInvalidDelta {
		t.Fatalf("want ErrInvalidDelta, got %v", err)
	}
}

func TestMovementType_NormalizeDelta(t *testing.T) {
	if d, err := inv.MovementSale.NormalizeDelta(2); err != nil || d != -2 {
		t.Fatalf("want -2 nil, got %v %v", d, err)
	}
	if d, err := inv.MovementPurchase.NormalizeDelta(-2); err != nil || d != 2 {
		t.Fatalf("want 2 nil, got %v %v", d, err)
	}
	if _, err := inv.MovementType("bad").NormalizeDelta(1); err != inv.ErrInvalidMovementType {
		t.Fatalf("want ErrInvalidMovementType, got %v", err)
	}
}
