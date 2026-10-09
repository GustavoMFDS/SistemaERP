package application

import (
	"testing"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func TestFiscalItemNetValuesAllocatesGlobalDiscountDeterministically(t *testing.T) {
	items := []sales.SaleItem{
		{
			ID:            "item-1",
			Qty:           platform.NewQuantityMilli(1000),
			UnitPrice:     platform.NewMoneyCents(1000),
			DiscountValue: platform.NewMoneyCents(100),
		},
		{
			ID:            "item-2",
			Qty:           platform.NewQuantityMilli(1000),
			UnitPrice:     platform.NewMoneyCents(2000),
			DiscountValue: platform.NewMoneyCents(0),
		},
	}
	sale := sales.Sale{
		DiscountValue: platform.NewMoneyCents(103), // R$1.00 item + R$0.03 global.
	}

	got, err := fiscalItemNetValues(sale, items)
	if err != nil {
		t.Fatalf("fiscalItemNetValues: %v", err)
	}
	if got["item-1"].DBString() != "9.00" {
		t.Fatalf("item-1 net=%s, want 9.00", got["item-1"].DBString())
	}
	if got["item-2"].DBString() != "19.97" {
		t.Fatalf("item-2 net=%s, want 19.97", got["item-2"].DBString())
	}
	total, err := got["item-1"].AddChecked(got["item-2"])
	if err != nil {
		t.Fatal(err)
	}
	if total.DBString() != "28.97" {
		t.Fatalf("net total=%s, want 28.97", total.DBString())
	}
}

func TestFiscalItemNetValuesRejectsImpossibleDiscount(t *testing.T) {
	items := []sales.SaleItem{{
		ID:        "item-1",
		Qty:       platform.NewQuantityMilli(1000),
		UnitPrice: platform.NewMoneyCents(1000),
	}}
	sale := sales.Sale{DiscountValue: platform.NewMoneyCents(1001)}
	if _, err := fiscalItemNetValues(sale, items); err == nil {
		t.Fatal("expected discount above sale value to fail")
	}
}

func TestProportionalFloorDoesNotOverAllocate(t *testing.T) {
	share, err := proportionalFloor(3, 900, 2900)
	if err != nil {
		t.Fatal(err)
	}
	if share != 0 {
		t.Fatalf("share=%d, want 0", share)
	}
}
