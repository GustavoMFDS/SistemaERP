package handlers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/inventory/application"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

func stockReportCSV(items []inv.StockReportRow) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	writer.Comma = ';'
	if err := writer.Write([]string{"SKU", "Produto", "Unidade", "Saldo", "Estoque minimo", "Situacao do estoque", "Cadastro"}); err != nil {
		return nil, err
	}
	for _, item := range items {
		availability := "OK"
		if item.BelowLimit {
			availability = "Baixo"
		}
		status := "Ativo"
		if !item.Active {
			status = "Inativo"
		}
		if err := writer.Write([]string{
			safeSpreadsheetCell(item.SKU), safeSpreadsheetCell(item.Name),
			safeSpreadsheetCell(item.Unit),
			strings.ReplaceAll(item.Quantity, ".", ","),
			strings.ReplaceAll(item.Minimum, ".", ","),
			availability, status,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		return nil, writer.Error()
	}
	return buf.Bytes(), nil
}

func (h *InventoryHandler) ExportStockReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	if len(r.URL.Query()) > 0 {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "filtros nao suportados", nil)
		return
	}
	items, err := h.svc.StockReport(r.Context(), au.TenantID)
	switch {
	case errors.Is(err, application.ErrStockReportTooLarge):
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "mais de 5000 produtos; solicite relatorio segmentado", nil)
		return
	case err != nil:
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel gerar relatorio", nil)
		return
	}
	file, err := stockReportCSV(items)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "nao foi possivel gerar CSV", nil)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="relatorio-estoque-loja.csv"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file)
}
