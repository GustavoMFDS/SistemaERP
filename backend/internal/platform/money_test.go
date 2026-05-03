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
