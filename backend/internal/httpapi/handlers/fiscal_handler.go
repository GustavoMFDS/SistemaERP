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
	profile, err := h.svc.PrepareNFCeIssuerProfile(r.Context(), au.TenantID, req)
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
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfce_issuer.prepare", "company", au.TenantID, "success", map[string]any{
		"crt":       profile.CRT,
		"state":     profile.AddressState,
		"city_code": profile.AddressCityCode,
	})
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
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfce_config.prepare", "nfce_config", au.TenantID, "success", map[string]any{
		"environment": cfg.Environment,
		"series":      cfg.Series,
		"enabled":     false,
	})
	writeJSON(w, http.StatusOK, cfg)
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
	recordAudit(h.audit, r, au.TenantID, au.UserID, "fiscal.nfe_xml.generate", "invoice_xml_file", xmlID, "success", map[string]any{"invoice_id": invoiceID})
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
