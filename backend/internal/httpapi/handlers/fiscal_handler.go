package handlers

import (
	"net/http"
	"strconv"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	"github.com/go-chi/chi/v5"
)

type FiscalHandler struct {
	svc    *fiscapp.FiscalService
	audit  *audit.Service
	logger *slog.Logger
}

func NewFiscalHandler(svc *fiscapp.FiscalService, auditSvc *audit.Service, logger *slog.Logger) *FiscalHandler {
	return &FiscalHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *FiscalHandler) GenerateNFeXML(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req fiscapp.GenerateXMLRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	saleID, valid := normalizeUUID(req.SaleID)
	if !valid {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "sale_id invalido", nil)
		return
	}
	req.SaleID = saleID
	invoiceID, xmlID, err := h.svc.GenerateNFeXML(r.Context(), au.TenantID, au.UserID, req)
	if err != nil {
		status := http.StatusBadRequest
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrInvoiceAlreadyExists:
			status = http.StatusConflict
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrSaleNotFinalized:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"invoice_id": invoiceID, "xml_file_id": xmlID})
}

func (h *FiscalHandler) ListXML(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListXML(r.Context(), au.TenantID, limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao listar XML", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *FiscalHandler) DownloadXML(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	id, ok := requireUUID(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	name, content, err := h.svc.DownloadXML(r.Context(), au.TenantID, id)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", "arquivo nao encontrado", nil)
		return
	}
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfe_xml.download", "invoice_xml_file", id, "success", nil)
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
