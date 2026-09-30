package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type FiscalService struct {
	uow      db.UnitOfWork
	fiscal   FiscalRepository
	sales    SalesRepository
	products ProductsRepository
	nfe      NFeProvider
	audit    *audit.Service
	validate *validator.Validate
	logger   *slog.Logger
}

type GenerateXMLRequest struct {
	SaleID string `json:"sale_id" validate:"required"`
}

type PrepareNFCeConfigRequest struct {
	Environment          string `json:"environment" validate:"required,oneof=homologation production"`
	Series               int    `json:"series" validate:"min=0,max=889"`
	CSCID                string `json:"csc_id" validate:"omitempty,max=32"`
	CSCSecretRef         string `json:"csc_secret_ref" validate:"omitempty,max=500"`
	CertificateSecretRef string `json:"certificate_secret_ref" validate:"required,max=500"`
}

type PrepareNFCeIssuerRequest struct {
	IE                  string  `json:"ie" validate:"required,min=2,max=30"`
	CRT                 string  `json:"crt" validate:"required,oneof=1 2 3 4"`
	AddressStreet       string  `json:"address_street" validate:"required,min=2,max=200"`
	AddressNumber       string  `json:"address_number" validate:"required,min=1,max=30"`
	AddressComplement   *string `json:"address_complement" validate:"omitempty,max=100"`
	AddressNeighborhood string  `json:"address_neighborhood" validate:"required,min=2,max=120"`
	AddressCity         string  `json:"address_city" validate:"required,min=2,max=120"`
	AddressCityCode     string  `json:"address_city_code" validate:"required,numeric,len=7"`
	AddressState        string  `json:"address_state" validate:"required,alpha,len=2"`
	AddressZIP          string  `json:"address_zip" validate:"required,numeric,len=8"`
}

func NewFiscalService(uow db.UnitOfWork, fiscal FiscalRepository, salesRepo SalesRepository, productsRepo ProductsRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *FiscalService {
	return &FiscalService{uow: uow, fiscal: fiscal, sales: salesRepo, products: productsRepo, nfe: nil, audit: auditSvc, validate: v, logger: logger}
}

func NewFiscalServiceWithProvider(
	uow db.UnitOfWork,
	fiscal FiscalRepository,
	salesRepo SalesRepository,
	productsRepo ProductsRepository,
	nfeProvider NFeProvider,
	auditSvc *audit.Service,
	v *validator.Validate,
	logger *slog.Logger,
) *FiscalService {
	return &FiscalService{uow: uow, fiscal: fiscal, sales: salesRepo, products: productsRepo, nfe: nfeProvider, audit: auditSvc, validate: v, logger: logger}
}

func (s *FiscalService) GenerateNFeXML(ctx context.Context, tenantID string, actorUserID string, req GenerateXMLRequest) (invoiceID, xmlID string, err error) {
	if err := s.validate.Struct(req); err != nil {
		return "", "", common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, req.SaleID)
	if err != nil {
		return "", "", common.ErrNotFound
	}
	if sale.Status != "finalized" {
		return "", "", common.ErrSaleNotFinalized
	}

	exists, err := s.fiscal.ExistsInvoiceForSale(ctx, tx, tenantID, req.SaleID)
	if err != nil {
		return "", "", err
	}
	if exists {
		return "", "", common.ErrInvoiceAlreadyExists
	}

	companyID := tenantID
	if s.nfe == nil {
		return "", "", fmt.Errorf("nfe provider not configured")
	}

	prodIDs := uniqueProductIDs(items)
	prodMap, err := s.products.GetManyByIDs(ctx, tx, tenantID, prodIDs)
	if err != nil {
		return "", "", err
	}

	xmlBytes, fileName, err := s.nfe.GenerateNFeXML(ctx, sale, items, prodMap)
	if err != nil {
		return "", "", err
	}

	sha := sha256.Sum256(xmlBytes)
	shaHex := hex.EncodeToString(sha[:])
	actor := actorUserID
	invID, xmlFileID, err := s.fiscal.CreateInvoiceWithXML(ctx, tx, tenantID, req.SaleID, companyID, &actor, fileName, xmlBytes, shaHex)
	if err != nil {
		return "", "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfe_xml.generate",
		ResourceType: "invoice_xml_file", ResourceID: xmlFileID, Outcome: "success",
		Metadata: map[string]any{"invoice_id": invID},
	}); err != nil {
		return "", "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return invID, xmlFileID, nil
}

func (s *FiscalService) ListXML(ctx context.Context, tenantID string, limit, offset int) ([]fisc.XMLFile, int, error) {
	return s.fiscal.ListXML(ctx, tenantID, limit, offset)
}

func (s *FiscalService) DownloadXML(ctx context.Context, tenantID string, id string) (string, []byte, error) {
	return s.fiscal.GetXMLContent(ctx, tenantID, id)
}

func uniqueProductIDs(items []sales.SaleItem) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.ProductID == "" {
			continue
		}
		if _, ok := seen[it.ProductID]; ok {
			continue
		}
		seen[it.ProductID] = struct{}{}
		out = append(out, it.ProductID)
	}
	return out
}

func (s *FiscalService) NFCeReadiness(ctx context.Context, tenantID string) (fisc.NFCeReadiness, error) {
	readiness, err := s.fiscal.GetNFCeReadiness(ctx, tenantID)
	if err != nil {
		return fisc.NFCeReadiness{}, err
	}
	profile, err := s.fiscal.GetNFCeIssuerProfile(ctx, tenantID)
	if err != nil {
		return fisc.NFCeReadiness{}, err
	}
	if err := fisc.ValidateCNPJ(profile.CNPJ); err != nil {
		readiness.IssuerIdentityConfigured = false
		readiness.ReadyForHomologationData = false
		if !containsReadinessReason(readiness.BlockingReasons, "issuer_identity") {
			readiness.BlockingReasons = append(readiness.BlockingReasons, "issuer_identity")
		}
	}
	return readiness, nil
}

func containsReadinessReason(reasons []string, target string) bool {
	for _, reason := range reasons {
		if reason == target {
			return true
		}
	}
	return false
}

func (s *FiscalService) ReserveNFCeDraft(
	ctx context.Context,
	tenantID, actorUserID, saleID string,
	issuedAt time.Time,
) (fisc.NFCeReservation, bool, error) {
	saleID = strings.TrimSpace(saleID)
	if issuedAt.IsZero() || s.validate.Var(saleID, "required,uuid") != nil {
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, _, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, saleID)
	if err != nil {
		return fisc.NFCeReservation{}, false, common.ErrNotFound
	}
	if sale.Status != "finalized" {
		return fisc.NFCeReservation{}, false, common.ErrSaleNotFinalized
	}

	existing, err := s.fiscal.GetNFCeReservationBySale(ctx, tx, tenantID, saleID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return existing, false, nil
	}
	if err != common.ErrNotFound {
		return fisc.NFCeReservation{}, false, err
	}

	exists, err := s.fiscal.ExistsInvoiceForSale(ctx, tx, tenantID, saleID)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	if exists {
		return fisc.NFCeReservation{}, false, common.ErrInvoiceAlreadyExists
	}

	readiness, err := s.NFCeReadiness(ctx, tenantID)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	if !readiness.ReadyForHomologationData || readiness.Environment != "homologation" {
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}

	reservationContext, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	issuerReq := PrepareNFCeIssuerRequest{
		IE: reservationContext.Issuer.IE,
		CRT: reservationContext.Issuer.CRT,
		AddressStreet: reservationContext.Issuer.AddressStreet,
		AddressNumber: reservationContext.Issuer.AddressNumber,
		AddressComplement: reservationContext.Issuer.AddressComplement,
		AddressNeighborhood: reservationContext.Issuer.AddressNeighborhood,
		AddressCity: reservationContext.Issuer.AddressCity,
		AddressCityCode: reservationContext.Issuer.AddressCityCode,
		AddressState: reservationContext.Issuer.AddressState,
		AddressZIP: reservationContext.Issuer.AddressZIP,
	}
	if s.validate.Struct(issuerReq) != nil || fisc.ValidateCNPJ(reservationContext.Issuer.CNPJ) != nil {
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}
	if reservationContext.Config.Environment != "homologation" ||
		!reservationContext.Config.CSCReferenceConfigured ||
		!reservationContext.Config.CertificateReferenceConfigured {
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}

	numericCode, err := fisc.GenerateNFCeNumericCode()
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	number, err := s.fiscal.ReserveNextNFCeNumber(
		ctx, tx, tenantID, reservationContext.Config.Series,
	)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}

	accessKey, err := fisc.BuildNFCeAccessKey(fisc.NFCeAccessKeyInput{
		UF: reservationContext.Issuer.AddressState,
		IssuedAt: issuedAt,
		CNPJ: reservationContext.Issuer.CNPJ,
		Series: reservationContext.Config.Series,
		Number: number,
		NumericCode: numericCode,
		EmissionType: fisc.NFCeNormalEmissionType,
	})
	if err != nil {
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}

	reservation := fisc.NFCeReservation{
		SaleID: saleID,
		Status: "reserved",
		Model: fisc.NFCeModel,
		Series: reservationContext.Config.Series,
		DocumentNumber: number,
		Environment: reservationContext.Config.Environment,
		AccessKey: accessKey,
		EmissionType: fisc.NFCeNormalEmissionType,
		NumericCode: numericCode,
		CheckDigit: int(accessKey[len(accessKey)-1] - '0'),
		IssuedAt: issuedAt,
	}
	invoiceID, err := s.fiscal.CreateNFCeReservation(
		ctx, tx, tenantID, actorUserID, reservation,
	)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	reservation.InvoiceID = invoiceID

	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce.reserve",
		ResourceType: "invoice", ResourceID: invoiceID, Outcome: "success",
		Metadata: map[string]any{
			"sale_id": saleID,
			"model": reservation.Model,
			"series": reservation.Series,
			"document_number": reservation.DocumentNumber,
			"environment": reservation.Environment,
			"emission_type": reservation.EmissionType,
		},
	}); err != nil {
		return fisc.NFCeReservation{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	return reservation, true, nil
}

func (s *FiscalService) GetNFCeConfig(ctx context.Context, tenantID string) (fisc.NFCeConfig, error) {
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
}

func (s *FiscalService) GetNFCeIssuerProfile(ctx context.Context, tenantID string) (fisc.NFCeIssuerProfile, error) {
	return s.fiscal.GetNFCeIssuerProfile(ctx, tenantID)
}

func (s *FiscalService) PrepareNFCeIssuerProfile(
	ctx context.Context,
	tenantID string,
	actorUserID string,
	req PrepareNFCeIssuerRequest,
) (fisc.NFCeIssuerProfile, error) {
	req.IE = strings.TrimSpace(req.IE)
	req.CRT = strings.TrimSpace(req.CRT)
	req.AddressStreet = strings.TrimSpace(req.AddressStreet)
	req.AddressNumber = strings.TrimSpace(req.AddressNumber)
	req.AddressNeighborhood = strings.TrimSpace(req.AddressNeighborhood)
	req.AddressCity = strings.TrimSpace(req.AddressCity)
	req.AddressCityCode = strings.TrimSpace(req.AddressCityCode)
	req.AddressState = strings.ToUpper(strings.TrimSpace(req.AddressState))
	req.AddressZIP = strings.TrimSpace(req.AddressZIP)
	if req.AddressComplement != nil {
		value := strings.TrimSpace(*req.AddressComplement)
		if value == "" {
			req.AddressComplement = nil
		} else {
			req.AddressComplement = &value
		}
	}
	if err := s.validate.Struct(req); err != nil {
		return fisc.NFCeIssuerProfile{}, common.ErrValidation
	}
	if !fisc.IsValidUF(req.AddressState) {
		return fisc.NFCeIssuerProfile{}, common.ErrValidation
	}

	profile := fisc.NFCeIssuerProfile{
		TenantID:            tenantID,
		IE:                  req.IE,
		CRT:                 req.CRT,
		AddressStreet:       req.AddressStreet,
		AddressNumber:       req.AddressNumber,
		AddressComplement:   req.AddressComplement,
		AddressNeighborhood: req.AddressNeighborhood,
		AddressCity:         req.AddressCity,
		AddressCityCode:     req.AddressCityCode,
		AddressState:        req.AddressState,
		AddressZIP:          req.AddressZIP,
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fiscal.UpdateNFCeIssuerProfile(ctx, tx, tenantID, profile); err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce_issuer.prepare",
		ResourceType: "company", ResourceID: tenantID, Outcome: "success",
		Metadata: map[string]any{
			"crt": profile.CRT,
			"state": profile.AddressState,
			"city_code": profile.AddressCityCode,
		},
	}); err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	return s.fiscal.GetNFCeIssuerProfile(ctx, tenantID)
}

func (s *FiscalService) PrepareNFCeConfig(
	ctx context.Context,
	tenantID string,
	actorUserID string,
	req PrepareNFCeConfigRequest,
) (fisc.NFCeConfig, error) {
	req.Environment = strings.ToLower(strings.TrimSpace(req.Environment))
	req.CSCID = strings.TrimSpace(req.CSCID)
	req.CSCSecretRef = strings.TrimSpace(req.CSCSecretRef)
	req.CertificateSecretRef = strings.TrimSpace(req.CertificateSecretRef)
	if err := s.validate.Struct(req); err != nil {
		return fisc.NFCeConfig{}, common.ErrValidation
	}
	if (req.CSCID == "") != (req.CSCSecretRef == "") {
		return fisc.NFCeConfig{}, common.ErrValidation
	}

	var cscID *string
	var cscSecretRef *string
	if req.CSCID != "" {
		value := req.CSCID
		cscID = &value
	}
	if req.CSCSecretRef != "" {
		value := req.CSCSecretRef
		cscSecretRef = &value
	}
	certificateSecretRef := req.CertificateSecretRef
	cfg := fisc.NFCeConfig{
		TenantID:             tenantID,
		Enabled:              false,
		Environment:          req.Environment,
		Series:               req.Series,
		CSCID:                cscID,
		CSCSecretRef:         cscSecretRef,
		CertificateSecretRef: &certificateSecretRef,
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeConfig{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fiscal.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, cfg); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce_config.prepare",
		ResourceType: "nfce_config", ResourceID: tenantID, Outcome: "success",
		Metadata: map[string]any{
			"environment": cfg.Environment,
			"series": cfg.Series,
			"enabled": false,
		},
	}); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeConfig{}, err
	}
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
}
