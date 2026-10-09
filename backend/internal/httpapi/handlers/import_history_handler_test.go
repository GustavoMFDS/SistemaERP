package handlers

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/inventory/application"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

func TestImportHistoryQueryRejectsMalformedDateAndPage(t *testing.T) {
	invalid := []string{
		"/?from=2026-02-30",
		"/?to=2026-13-01",
		"/?from=2026-10-09&to=2026-10-08",
		"/?from=2024-01-01&to=2026-10-08",
		"/?from=2026-10-08&from=2026-10-09",
		"/?to=",
		"/?limit=51",
		"/?offset=-1",
		"/?limit=10&limit=20",
	}
	for _, raw := range invalid {
		r := httptest.NewRequest(http.MethodGet, raw, nil)
		_, _, pageErr := readImportHistoryPage(r)
		_, dateErr := readImportHistoryFilter(r)
		if pageErr == nil && dateErr == nil {
			t.Errorf("accepted invalid history query: %s", raw)
		}
	}
	valid := httptest.NewRequest(http.MethodGet, "/?from=2026-10-08&to=2026-10-08&limit=10&offset=0", nil)
	if _, _, err := readImportHistoryPage(valid); err != nil {
		t.Fatal(err)
	}
	if filter, err := readImportHistoryFilter(valid); err != nil || filter.From != "2026-10-08" || filter.To != "2026-10-08" {
		t.Fatalf("valid calendar bounds: %+v err=%v", filter, err)
	}
}

func TestHistoryCSVDoesNotExecuteFormulasOrExposePayload(t *testing.T) {
	items := []inv.ImportBatchEntry{
		{
			BatchID: "00000000-0000-4000-8000-000000000001",
			ActorName: " \t=HYPERLINK(\"https://example.test\",\"x\")",
			ItemCount: 2,
			CreatedAt: time.Date(2026, 10, 8, 19, 0, 0, 0, time.UTC),
		},
		{
			BatchID: "00000000-0000-4000-8000-000000000002",
			ActorName: "+SOMA(1;1)",
			ItemCount: 1,
			CreatedAt: time.Date(2026, 10, 7, 19, 0, 0, 0, time.UTC),
		},
	}
	response := httptest.NewRecorder()
	writeImportHistoryCSV(response, httptest.NewRequest(http.MethodGet, "/", nil), "products", items, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected CSV status: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "historico-importacao-produtos.csv") {
		t.Fatalf("unsafe export filename: %s", response.Header().Get("Content-Disposition"))
	}
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("download is missing no-store or nosniff")
	}
	body := response.Body.String()
	if !strings.HasPrefix(body, "\uFEFF") {
		t.Fatal("UTF-8 BOM missing for spreadsheet import")
	}
	if strings.Contains(body, "request_hash") || strings.Contains(body, "idem_key") || strings.Contains(body, "cost_price") {
		t.Fatalf("CSV exported internal data: %s", body)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(body, "\uFEFF")))
	reader.Comma = ';'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || len(rows[1]) != 4 {
		t.Fatalf("wrong number of sanitized CSV columns: %#v", rows)
	}
	if !strings.HasPrefix(rows[1][1], "'") || !strings.HasPrefix(rows[2][1], "'") {
		t.Fatalf("CSV injection was not neutralized: %#v", rows)
	}
	if rows[1][2] != "2" {
		t.Fatalf("count missing: %#v", rows[1])
	}
}

func TestHistoryCSVRejectsOversizedExportBeforeSendingFile(t *testing.T) {
	response := httptest.NewRecorder()
	writeImportHistoryCSV(response, httptest.NewRequest(http.MethodGet, "/", nil), "opening-stock", nil, application.ErrImportHistoryExportTooLarge)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", response.Code)
	}
	if response.Header().Get("Content-Disposition") != "" {
		t.Fatal("a rejected export must not be a file download")
	}
}
