package handlers

import (
	"net/http"
	"strconv"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/go-chi/chi/v5"
)

type reserveNFCeRequest struct {
	SaleID   string `json:"sale_id"`
	IssuedAt string `json:"issued_at"`
}

type setNFCeTransmissionRequest struct {
	Enabled bool `json:"enabled"`
}

type prepareLegacyTaxRequest struct {
	CalculationVersion string `json:"calculation_version"`
}

type prepareRegularIBSCBSTaxRequest struct {
	CalculationVersion string                        `json:"calculation_version"`
	Base               platform.Money                `json:"base"`
	IBSUF              fisc.RegularTaxComponentInput `json:"ibs_uf"`
	IBSMunicipal       fisc.RegularTaxComponentInput `json:"ibs_municipal"`
	CBS                fisc.RegularTaxComponentInput `json:"cbs"`
}

type FiscalHandler struct {
	svc    *fiscapp.FiscalService
	audit  *audit.Service
	logger *slog.Logger
}

func NewFiscalHandler(svc *fiscapp.FiscalService, auditSvc *audit.Service, logger *slog.Logger) *FiscalHandler {
	return &FiscalHandler{svc: svc, audit: auditSvc, logger: logger}
}

func (h *FiscalHandler) GetNFCeIssuerProfile(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	profile, err := h.svc.GetNFCeIssuerProfile(r.Context(), au.TenantID)
	if err != nil {
		if err == common.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "emitente nao encontrado", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar emitente NFC-e", nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *FiscalHandler) PrepareNFCeIssuerProfile(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req fiscapp.PrepareNFCeIssuerRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	profile, err := h.svc.PrepareNFCeIssuerProfile(r.Context(), au.TenantID, au.UserID, req)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *FiscalHandler) GetNFCeConfig(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	cfg, err := h.svc.GetNFCeConfig(r.Context(), au.TenantID)
	if err != nil {
		if err == common.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "configuracao NFC-e nao encontrada", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao consultar configuracao NFC-e", nil)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *FiscalHandler) PrepareNFCeConfig(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req fiscapp.PrepareNFCeConfigRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	cfg, err := h.svc.PrepareNFCeConfig(r.Context(), au.TenantID, au.UserID, req)
	if err != nil {
		status := http.StatusInternalServerError
		if err == common.ErrValidation {
			status = http.StatusUnprocessableEntity
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *FiscalHandler) SetNFCeProductionTransmission(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req setNFCeTransmissionRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	cfg, err := h.svc.SetNFCeProductionTransmission(
		r.Context(), au.TenantID, au.UserID, req.Enabled,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrFiscalNotReady, common.ErrConflict:
			status = http.StatusConflict
		case common.ErrNotFound:
			status = http.StatusNotFound
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *FiscalHandler) GetProductFiscalProfile(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	productID := chi.URLParam(r, "productID")
	profile, err := h.svc.GetProductFiscalProfile(r.Context(), au.TenantID, productID)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *FiscalHandler) PrepareProductFiscalProfile(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	productID := chi.URLParam(r, "productID")
	var req fiscapp.PrepareProductFiscalProfileRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	profile, err := h.svc.PrepareProductFiscalProfile(
		r.Context(), au.TenantID, au.UserID, productID, req,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *FiscalHandler) ReserveNFCeDraft(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	var req reserveNFCeRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	issuedAt, err := time.Parse(time.RFC3339, req.IssuedAt)
	if err != nil {
		writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "issued_at deve usar RFC3339 com fuso horario", nil)
		return
	}
	reservation, created, err := h.svc.ReserveNFCeDraft(
		r.Context(), au.TenantID, au.UserID, req.SaleID, issuedAt,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrInvoiceAlreadyExists, common.ErrSaleNotFinalized, common.ErrFiscalNotReady:
			status = http.StatusConflict
		case common.ErrFiscalSequenceExhausted:
			status = http.StatusServiceUnavailable
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{
		"reservation": reservation,
		"created":     created,
	})
}

func (h *FiscalHandler) PrepareLegacyOnlyTaxCalculation(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	saleItemID := chi.URLParam(r, "saleItemID")
	var req prepareLegacyTaxRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	calculation, err := h.svc.PrepareLegacyOnlyTaxCalculation(
		r.Context(), au.TenantID, au.UserID, invoiceID, saleItemID, req.CalculationVersion,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusCreated, calculation)
}

func (h *FiscalHandler) PrepareRegularIBSCBSCalculation(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	saleItemID := chi.URLParam(r, "saleItemID")
	var req prepareRegularIBSCBSTaxRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	calculation, err := h.svc.PrepareRegularIBSCBSCalculation(
		r.Context(),
		au.TenantID,
		au.UserID,
		fiscapp.PrepareRegularIBSCBSCalculationInput{
			InvoiceID:          invoiceID,
			SaleItemID:         saleItemID,
			CalculationVersion: req.CalculationVersion,
			Base:               req.Base,
			IBSUF:              req.IBSUF,
			IBSMunicipal:       req.IBSMunicipal,
			CBS:                req.CBS,
		},
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusCreated, calculation)
}

func (h *FiscalHandler) CancelNFCe(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	var req fiscapp.CancelNFCeRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	outcome, err := h.svc.CancelNFCe(
		r.Context(), au.TenantID, au.UserID, invoiceID, req,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	status := http.StatusOK
	if outcome.Pending() {
		status = http.StatusAccepted
	}
	writeJSON(w, status, outcome)
}

func (h *FiscalHandler) AuthorizeNFCe(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	outcome, err := h.svc.AuthorizeNFCe(
		r.Context(), au.TenantID, au.UserID, invoiceID,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	status := http.StatusOK
	if outcome.Pending() {
		status = http.StatusAccepted
	}
	writeJSON(w, status, outcome)
}

func (h *FiscalHandler) AuthorizeNFCeHomologation(w http.ResponseWriter, r *http.Request) {
	h.AuthorizeNFCe(w, r)
}

func (h *FiscalHandler) CancelNFCeHomologation(w http.ResponseWriter, r *http.Request) {
	h.CancelNFCe(w, r)
}

func (h *FiscalHandler) SignNFCeReserved(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	xmlID, fileName, err := h.svc.SignNFCeReserved(
		r.Context(), au.TenantID, au.UserID, invoiceID,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"invoice_id": invoiceID,
		"xml_id":     xmlID,
		"file_name":  fileName,
		"status":     "signed",
	})
}

func (h *FiscalHandler) PreviewNFCeXMLCandidate(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	content, fileName, err := h.svc.BuildNFCeUnsignedCandidate(
		r.Context(), au.TenantID, invoiceID,
	)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		case common.ErrConflict, common.ErrFiscalNotReady:
			status = http.StatusConflict
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "inline; filename=\""+fileName+"\"")
	w.Header().Set("X-Fiscal-Document-State", "unsigned-candidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (h *FiscalHandler) ListInvoiceTaxCalculations(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	invoiceID := chi.URLParam(r, "invoiceID")
	items, err := h.svc.GetInvoiceTaxCalculations(r.Context(), au.TenantID, invoiceID)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case common.ErrValidation:
			status = http.StatusUnprocessableEntity
		case common.ErrNotFound:
			status = http.StatusNotFound
		}
		writeError(w, r, status, errorCodeForStatus(status), friendlyErrorMessage(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *FiscalHandler) NFCeReadiness(w http.ResponseWriter, r *http.Request) {
	au, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "nao autenticado", nil)
		return
	}
	readiness, err := h.svc.NFCeReadiness(r.Context(), au.TenantID)
	if err != nil {
		if err == common.ErrNotFound {
			writeError(w, r, http.StatusNotFound, "not_found", "emitente nao encontrado", nil)
			return
		}
		writeError(w, r, http.StatusInternalServerError, "internal_error", "erro ao validar prontidao NFC-e", nil)
		return
	}
	writeJSON(w, http.StatusOK, readiness)
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
	id := chi.URLParam(r, "id")
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
