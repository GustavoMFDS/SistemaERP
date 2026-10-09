package handlers

import (
	"encoding/csv"
	"strings"
	"testing"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

func TestStockReportCSVEscapesSpreadsheetFormulasAndOmitsFinance(t *testing.T) {
	result, err := stockReportCSV([]inv.StockReportRow{{
		SKU:        "=HYPERLINK(\"https://test.invalid\")",
		Name:       " \t+SUM(1,1)",
		Unit:       "un",
		Quantity:   "2.500",
		Minimum:    "3.000",
		Active:     true,
		BelowLimit: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(result), "\uFEFF") {
		t.Fatal("CSV should use UTF-8 BOM")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(result), "\uFEFF")))
	reader.Comma = ';'
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || len(records[1]) != 7 {
		t.Fatalf("unexpected CSV rows: %+v", records)
	}
	if !strings.HasPrefix(records[1][0], "'") || !strings.HasPrefix(records[1][1], "'") {
		t.Fatalf("spreadsheet formula was not neutralized: %+v", records[1])
	}
	if records[1][3] != "2,500" || records[1][4] != "3,000" ||
		records[1][5] != "Baixo" || records[1][6] != "Ativo" {
		t.Fatalf("stock columns differ from report: %+v", records[1])
	}
	for _, forbidden := range []string{"cost_price", "price_cash", "profit_estimated", "tenant_id"} {
		if strings.Contains(string(result), forbidden) {
			t.Fatalf("financial or tenant data leaked: %s", forbidden)
		}
	}
}


func TestSafeSpreadsheetCellUnicodeFormulaProtection(t *testing.T) {
	for _, value := range []string{
		"\u00A0=SUM(1,2)",
		"\u200B@cmd",
		"\uFEFF+1+1",
		"\n\t-SUM(10,2)",
		"\t=HYPERLINK(\"https://example.test\")",
	} {
		if got := safeSpreadsheetCell(value); got != "'"+value {
			t.Errorf("untrusted spreadsheet text not neutralized: %q -> %q", value, got)
		}
	}
	for _, value := range []string{"Produto regular", "012345", "Arroz 5 kg"} {
		if got := safeSpreadsheetCell(value); got != value {
			t.Errorf("ordinary value was unexpectedly changed: %q -> %q", value, got)
		}
	}
}
