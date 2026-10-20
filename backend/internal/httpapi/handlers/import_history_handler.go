package handlers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/example/sistemaemgo/internal/modules/inventory/application"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

// Query params are bounded to limit=1..50, offset=0..5000 and optional
// calendar dates from/to (YYYY-MM-DD, Sao Paulo time zone).
func readImportHistoryPage(r *http.Request) (int, int, error) {
	limit, offset := 20, 0
	query := r.URL.Query()
	if values, exists := query["limit"]; exists {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, common.ErrValidation
		}
		number, err := strconv.Atoi(values[0])
		if err != nil || number < 1 || number > 50 {
			return 0, 0, common.ErrValidation
		}
		limit = number
	}
	if values, exists := query["offset"]; exists {
		if len(values) != 1 || values[0] == "" {
			return 0, 0, common.ErrValidation
		}
		number, err := strconv.Atoi(values[0])
		if err != nil || number < 0 || number > 5000 {
			return 0, 0, common.ErrValidation
		}
		offset = number
	}
	return limit, offset, nil
}

func readImportHistoryFilter(r *http.Request) (application.ImportHistoryFilter, error) {
	var filter application.ImportHistoryFilter
	query := r.URL.Query()
	if values, exists := query["from"]; exists {
		if len(values) != 1 || values[0] == "" {
			return filter, common.ErrValidation
		}
		filter.From = values[0]
	}
	if values, exists := query["to"]; exists {
		if len(values) != 1 || values[0] == "" {
			return filter, common.ErrValidation
		}
		filter.To = values[0]
	}
	return filter, application.ValidateImportHistoryFilter(filter)
}

func writeImportHistory(w http.ResponseWriter, r *http.Request, result application.ImportHistoryPage, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "filtros ou pagina de historico invalidos", nil)
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel consultar o historico", nil)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *ProductsHandler) ListImportHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, offset, err := readImportHistoryPage(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	filter, err := readImportHistoryFilter(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	page, err := h.svc.ListImportHistory(r.Context(), au.TenantID, limit, offset, filter)
	writeImportHistory(w, r, page, err)
}

func (h *InventoryHandler) ListOpeningStockHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, offset, err := readImportHistoryPage(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	filter, err := readImportHistoryFilter(r)
	if err != nil {
		writeImportHistory(w, r, application.ImportHistoryPage{}, err)
		return
	}
	page, err := h.svc.ListOpeningStockHistory(r.Context(), au.TenantID, limit, offset, filter)
	writeImportHistory(w, r, page, err)
}

// Even if a malicious user profile contains a spreadsheet formula (including
// leading whitespace), the CSV export must not execute it in Excel/Calc.
func safeSpreadsheetCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) ||
			r == '\uFEFF' || r == '\u200B' || r == '\u2060'
	})
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	if value != "" {
		first, _ := utf8.DecodeRuneInString(value)
		if unicode.IsControl(first) {
			return "'" + value
		}
	}
	return value
}

func writeImportHistoryCSV(w http.ResponseWriter, r *http.Request, kind string, items []inv.ImportBatchEntry, err error) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case errors.Is(err, common.ErrValidation):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "periodo de historico invalido", nil)
		return
	case errors.Is(err, application.ErrImportHistoryExportTooLarge):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "mais de 1000 lotes; escolha um periodo menor para exportar", nil)
		return
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel exportar o historico", nil)
		return
	}
	// Assemble everything in memory before writing headers. No partial success
	// downloads on database errors. Bounded by 1,000 sanitized metadata rows.
	var buffer bytes.Buffer
	buffer.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	writer := csv.NewWriter(&buffer)
	writer.Comma = ';'
	if err := writer.Write([]string{"Data (UTC)", "Responsável", "Produtos", "Identificador do lote"}); err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro de CSV", nil)
		return
	}
	for _, item := range items {
		if err := writer.Write([]string{
			item.CreatedAt.UTC().Format(time.RFC3339),
			safeSpreadsheetCell(item.ActorName),
			strconv.Itoa(item.ItemCount),
			item.BatchID,
		}); err != nil {
			writeError(w, r, http.StatusInternalServerError, "internal_error", "erro de CSV", nil)
			return
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro de CSV", nil)
		return
	}
	filename := "historico-importacao-produtos.csv"
	if kind == "opening-stock" {
		filename = "historico-estoque-inicial.csv"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}

func (h *ProductsHandler) ExportImportHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	if r.URL.Query().Has("offset") || r.URL.Query().Has("limit") {
		writeImportHistoryCSV(w, r, "products", nil, common.ErrValidation)
		return
	}
	filter, err := readImportHistoryFilter(r)
	if err != nil {
		writeImportHistoryCSV(w, r, "products", nil, err)
		return
	}
	items, err := h.svc.ExportImportHistory(r.Context(), au.TenantID, filter)
	writeImportHistoryCSV(w, r, "products", items, err)
}

func (h *InventoryHandler) ExportOpeningStockHistory(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	if r.URL.Query().Has("offset") || r.URL.Query().Has("limit") {
		writeImportHistoryCSV(w, r, "opening-stock", nil, common.ErrValidation)
		return
	}
	filter, err := readImportHistoryFilter(r)
	if err != nil {
		writeImportHistoryCSV(w, r, "opening-stock", nil, err)
		return
	}
	items, err := h.svc.ExportOpeningStockHistory(r.Context(), au.TenantID, filter)
	writeImportHistoryCSV(w, r, "opening-stock", items, err)
}
