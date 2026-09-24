package platform_test

import (
	"encoding/json"
	"testing"

	"github.com/example/sistemaemgo/internal/platform"
)

func TestMoneyParsingFormattingAndArithmetic(t *testing.T) {
	m, err := platform.ParseMoney("10.23")
	if err != nil {
		t.Fatalf("parse money: %v", err)
	}
	if m.Cents() != 1023 {
		t.Fatalf("want 1023 cents, got %d", m.Cents())
	}
	if got := m.DBString(); got != "10.23" {
		t.Fatalf("want 10.23, got %s", got)
	}

	price := platform.NewMoneyCents(199)
	qty := platform.NewQuantityMilli(3_000)
	if got := price.MulQty(qty); got.Cents() != 597 {
		t.Fatalf("want 597 cents, got %d", got.Cents())
	}
}

func TestMoneyRejectsInvalidInputs(t *testing.T) {
	for _, raw := range []string{"", "10.", "10.235", "abc", "1,23"} {
		if _, err := platform.ParseMoney(raw); err == nil {
			t.Fatalf("ParseMoney(%q) expected error", raw)
		}
	}
}

func TestQuantityParsingFormattingAndJSON(t *testing.T) {
	var q platform.Quantity
	if err := json.Unmarshal([]byte(`1.234`), &q); err != nil {
		t.Fatalf("unmarshal quantity: %v", err)
	}
	if q.Milli() != 1234 {
		t.Fatalf("want 1234 milli, got %d", q.Milli())
	}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal quantity: %v", err)
	}
	if string(b) != "1.234" {
		t.Fatalf("want 1.234, got %s", string(b))
	}
}

func TestQuantityRejectsInvalidInputs(t *testing.T) {
	for _, raw := range []string{"", "1.", "1.2349", "abc", "1,234"} {
		if _, err := platform.ParseQuantity(raw); err == nil {
			t.Fatalf("ParseQuantity(%q) expected error", raw)
		}
	}
}

func TestScaledTypesRejectDatabaseRangeOverflow(t *testing.T) {
	if _, err := platform.ParseMoney("10000000000.00"); err == nil {
		t.Fatal("expected numeric(12,2) money overflow to be rejected")
	}
	if _, err := platform.ParseMoney("-10000000000.00"); err == nil {
		t.Fatal("expected negative numeric(12,2) money overflow to be rejected")
	}
	if _, err := platform.ParseQuantity("100000000000.000"); err == nil {
		t.Fatal("expected numeric(14,3) quantity overflow to be rejected")
	}
}

func TestCheckedMoneyArithmeticRejectsOverflow(t *testing.T) {
	maxMoney, err := platform.ParseMoney("9999999999.99")
	if err != nil {
		t.Fatalf("parse max money: %v", err)
	}
	cent, err := platform.ParseMoney("0.01")
	if err != nil {
		t.Fatalf("parse cent: %v", err)
	}
	if _, err := maxMoney.AddChecked(cent); err == nil {
		t.Fatal("expected checked addition beyond numeric(12,2) to fail")
	}

	largeQty, err := platform.ParseQuantity("99999999999.999")
	if err != nil {
		t.Fatalf("parse max quantity: %v", err)
	}
	if _, err := maxMoney.MulQtyChecked(largeQty); err == nil {
		t.Fatal("expected checked money*quantity overflow to fail")
	}
}

func TestMulDivRoundUsesSafeIntermediate(t *testing.T) {
	got, err := platform.MulDivRound(9_000_000_000_000_000_000, 2, 3)
	if err != nil {
		t.Fatalf("MulDivRound: %v", err)
	}
	if got != 6_000_000_000_000_000_000 {
		t.Fatalf("unexpected rounded value: %d", got)
	}

	negative, err := platform.MulDivRound(-5, 1, 2)
	if err != nil {
		t.Fatalf("MulDivRound negative: %v", err)
	}
	if negative != -3 {
		t.Fatalf("expected halves away from zero, got %d", negative)
	}
}

func TestCheckedQuantityArithmeticRejectsOverflow(t *testing.T) {
	maxQty, err := platform.ParseQuantity("99999999999.999")
	if err != nil {
		t.Fatalf("parse max quantity: %v", err)
	}
	if _, err := maxQty.AddChecked(platform.NewQuantityMilli(1)); err == nil {
		t.Fatal("expected checked quantity addition beyond numeric(14,3) to fail")
	}

	minQty, err := platform.ParseQuantity("-99999999999.999")
	if err != nil {
		t.Fatalf("parse min quantity: %v", err)
	}
	if _, err := minQty.SubChecked(platform.NewQuantityMilli(1)); err == nil {
		t.Fatal("expected checked quantity subtraction beyond numeric(14,3) to fail")
	}
}
