package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type FiscalService struct {
	uow                db.UnitOfWork
	fiscal             FiscalRepository
	sales              SalesRepository
	products           ProductsRepository
	nfe                NFeProvider
	nfceDoc            NFCeDocumentBuilder
	nfceDANFE          NFCeDANFERenderer
	nfceSigner         NFCeXMLSigner
	nfceValidator      NFCeSchemaValidator
	nfceAuthorizer     NFCeRemoteAuthorizer
	nfceProcessed      NFCeProcessedDocumentBuilder
	nfceCancelBuilder  NFCeCancellationEventBuilder
	nfceCancelSigner   NFCeCancellationEventSigner
	nfceEventValidator NFCeSchemaValidator
	nfceCancelClient   NFCeRemoteCancellationClient
	nfceInutBuilder    NFCeInutilizationBuilder
	nfceInutSigner     NFCeInutilizationSigner
	nfceInutValidator  NFCeSchemaValidator
	nfceInutClient     NFCeRemoteInutilizationClient
	audit              *audit.Service
	validate           *validator.Validate
	logger             *slog.Logger
}

type GenerateXMLRequest struct {
	SaleID string `json:"sale_id" validate:"required"`
}

type PrepareNFCeConfigRequest struct {
	Environment          string `json:"environment" validate:"required,oneof=homologation production"`
	Series               int    `json:"series" validate:"min=0,max=889"`
	CSCID                string `json:"csc_id" validate:"omitempty,max=32"`
	CSCSecretRef         string `json:"csc_secret_ref" validate:"omitempty,max=500"`
	CertificateSecretRef string `json:"certificate_secret_ref" validate:"omitempty,max=500"`
}

type PrepareProductFiscalProfileRequest struct {
	CFOP                 string  `json:"cfop" validate:"required,numeric,len=4"`
	ICMSOrigin           string  `json:"icms_origin" validate:"required,numeric,len=1"`
	ICMSRegime           string  `json:"icms_regime" validate:"required,oneof=cst csosn"`
	ICMSCode             string  `json:"icms_code" validate:"required,numeric"`
	PISCST               string  `json:"pis_cst" validate:"required,numeric,len=2"`
	COFINSCST            string  `json:"cofins_cst" validate:"required,numeric,len=2"`
	IBSCBSCST            *string `json:"ibs_cbs_cst" validate:"omitempty,numeric,len=3"`
	IBSCBSClassification *string `json:"ibs_cbs_classification" validate:"omitempty,numeric,len=6"`
	ISCST                *string `json:"is_cst" validate:"omitempty,numeric,len=3"`
	ISClassification     *string `json:"is_classification" validate:"omitempty,numeric,len=6"`
	ReferenceVersion     string  `json:"reference_version" validate:"required,min=2,max=120"`
}

type PrepareRegularIBSCBSCalculationInput struct {
	InvoiceID          string                        `json:"invoice_id"`
	SaleItemID         string                        `json:"sale_item_id"`
	CalculationVersion string                        `json:"calculation_version"`
	Base               platform.Money                `json:"base"`
	IBSUF              fisc.RegularTaxComponentInput `json:"ibs_uf"`
	IBSMunicipal       fisc.RegularTaxComponentInput `json:"ibs_municipal"`
	CBS                fisc.RegularTaxComponentInput `json:"cbs"`
}

type CancelNFCeRequest struct {
	Justification string `json:"justification" validate:"required,min=15,max=255"`
}

type InutilizeNFCeNumbersRequest struct {
	Year          int    `json:"year"`
	Series        int    `json:"series"`
	StartNumber   int64  `json:"start_number"`
	EndNumber     int64  `json:"end_number"`
	Justification string `json:"justification" validate:"required,min=15,max=255"`
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

func (s *FiscalService) SetNFCeDocumentBuilder(builder NFCeDocumentBuilder) {
	s.nfceDoc = builder
}

func (s *FiscalService) SetNFCeDANFERenderer(renderer NFCeDANFERenderer) {
	s.nfceDANFE = renderer
}

func (s *FiscalService) SetNFCeXMLSigner(signer NFCeXMLSigner) {
	s.nfceSigner = signer
}

func (s *FiscalService) SetNFCeSchemaValidator(schemaValidator NFCeSchemaValidator) {
	s.nfceValidator = schemaValidator
}

func (s *FiscalService) SetNFCeProcessedDocumentBuilder(builder NFCeProcessedDocumentBuilder) {
 s.nfceProcessed = builder
}

func (s *FiscalService) SetNFCeRemoteAuthorizer(authorizer NFCeRemoteAuthorizer) {
	s.nfceAuthorizer = authorizer
}

func (s *FiscalService) SetNFCeCancellationBuilder(builder NFCeCancellationEventBuilder) {
	s.nfceCancelBuilder = builder
}

func (s *FiscalService) SetNFCeCancellationSigner(signer NFCeCancellationEventSigner) {
	s.nfceCancelSigner = signer
}

func (s *FiscalService) SetNFCeEventSchemaValidator(schemaValidator NFCeSchemaValidator) {
	s.nfceEventValidator = schemaValidator
}

func (s *FiscalService) SetNFCeRemoteCancellationClient(client NFCeRemoteCancellationClient) {
	s.nfceCancelClient = client
}

func (s *FiscalService) SetNFCeInutilizationBuilder(builder NFCeInutilizationBuilder) {
	s.nfceInutBuilder = builder
}

func (s *FiscalService) SetNFCeInutilizationSigner(signer NFCeInutilizationSigner) {
	s.nfceInutSigner = signer
}

func (s *FiscalService) SetNFCeInutilizationSchemaValidator(validator NFCeSchemaValidator) {
	s.nfceInutValidator = validator
}

func (s *FiscalService) SetNFCeRemoteInutilizationClient(client NFCeRemoteInutilizationClient) {
	s.nfceInutClient = client
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

// The only endpoint for distributing an authorized NFC-e must return
// a previously archived nfeProc. A signed standalone NFe is a draft,
// never an official downloadable authorized document.
func (s *FiscalService) DownloadAuthorizedNFCeProcessedXML(
	ctx context.Context, tenantID, invoiceID string,
) (string,[]byte,error) {
	if s.validate.Var(strings.TrimSpace(invoiceID),"required,uuid")!=nil {return "",nil,common.ErrValidation}
	name,content,storedHash,err:=s.fiscal.GetAuthorizedNFCeProcessedXML(ctx,tenantID,invoiceID)
	if err!=nil {return "",nil,err}
	sum:=sha256.Sum256(content)
	if hex.EncodeToString(sum[:])!=storedHash {return "",nil,common.ErrConflict}
	return name,content,nil
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
	return s.reserveNFCeDraftWithMode(
		ctx,
		tenantID,
		actorUserID,
		saleID,
		issuedAt,
		fisc.NFCeNormalEmissionType,
		nil,
		nil,
	)
}

func (s *FiscalService) ReserveNFCeOfflineContingency(
	ctx context.Context,
	tenantID, actorUserID, saleID string,
	issuedAt, contingencyStartedAt time.Time,
	justification string,
) (fisc.NFCeReservation, bool, error) {
	justification = strings.TrimSpace(justification)
	if contingencyStartedAt.IsZero() ||
		contingencyStartedAt.After(issuedAt) ||
		len([]rune(justification)) < 15 ||
		len([]rune(justification)) > 256 {
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}
	return s.reserveNFCeDraftWithMode(
		ctx,
		tenantID,
		actorUserID,
		saleID,
		issuedAt,
		fisc.NFCeOfflineContingencyEmissionType,
		&contingencyStartedAt,
		&justification,
	)
}

func (s *FiscalService) reserveNFCeDraftWithMode(
	ctx context.Context,
	tenantID, actorUserID, saleID string,
	issuedAt time.Time,
	emissionType int,
	contingencyStartedAt *time.Time,
	contingencyJustification *string,
) (fisc.NFCeReservation, bool, error) {
	saleID = strings.TrimSpace(saleID)
	if issuedAt.IsZero() || s.validate.Var(saleID, "required,uuid") != nil {
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}
	switch emissionType {
	case fisc.NFCeNormalEmissionType:
		if contingencyStartedAt != nil || contingencyJustification != nil {
			return fisc.NFCeReservation{}, false, common.ErrValidation
		}
	case fisc.NFCeOfflineContingencyEmissionType:
		if contingencyStartedAt == nil || contingencyStartedAt.IsZero() ||
			contingencyStartedAt.After(issuedAt) ||
			contingencyJustification == nil {
			return fisc.NFCeReservation{}, false, common.ErrValidation
		}
		reason := strings.TrimSpace(*contingencyJustification)
		if len([]rune(reason)) < 15 || len([]rune(reason)) > 256 {
			return fisc.NFCeReservation{}, false, common.ErrValidation
		}
		contingencyJustification = &reason
	default:
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, saleID)
	if err != nil {
		return fisc.NFCeReservation{}, false, common.ErrNotFound
	}
	if sale.Status != "finalized" {
		return fisc.NFCeReservation{}, false, common.ErrSaleNotFinalized
	}

	existing, err := s.fiscal.GetNFCeReservationBySale(ctx, tx, tenantID, saleID)
	if err == nil {
		if existing.EmissionType != emissionType {
			return fisc.NFCeReservation{}, false, common.ErrConflict
		}
		if emissionType == fisc.NFCeOfflineContingencyEmissionType {
			if existing.ContingencyStartedAt == nil ||
				contingencyStartedAt == nil ||
				!existing.ContingencyStartedAt.Equal(*contingencyStartedAt) ||
				existing.ContingencyJustification == nil ||
				contingencyJustification == nil ||
				strings.TrimSpace(*existing.ContingencyJustification) !=
					strings.TrimSpace(*contingencyJustification) {
				return fisc.NFCeReservation{}, false, common.ErrConflict
			}
		}
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

	reservationContext, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	issuerReq := PrepareNFCeIssuerRequest{
		IE:                  reservationContext.Issuer.IE,
		CRT:                 reservationContext.Issuer.CRT,
		AddressStreet:       reservationContext.Issuer.AddressStreet,
		AddressNumber:       reservationContext.Issuer.AddressNumber,
		AddressComplement:   reservationContext.Issuer.AddressComplement,
		AddressNeighborhood: reservationContext.Issuer.AddressNeighborhood,
		AddressCity:         reservationContext.Issuer.AddressCity,
		AddressCityCode:     reservationContext.Issuer.AddressCityCode,
		AddressState:        reservationContext.Issuer.AddressState,
		AddressZIP:          reservationContext.Issuer.AddressZIP,
	}
	if s.validate.Struct(issuerReq) != nil || fisc.ValidateCNPJ(reservationContext.Issuer.CNPJ) != nil {
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}
	if !reservationContext.Config.CertificateReferenceConfigured {
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}
	switch reservationContext.Config.Environment {
	case "homologation":
	case "production":
		if !reservationContext.Config.Enabled || !s.productionTransmissionStackReady() {
			return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
		}
	default:
		return fisc.NFCeReservation{}, false, common.ErrFiscalNotReady
	}

	snapshots, err := s.buildSaleItemFiscalSnapshots(
		ctx, tx, tenantID, saleID, items,
	)
	if err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	if err := s.fiscal.CreateSaleItemFiscalSnapshots(ctx, tx, tenantID, snapshots); err != nil {
		return fisc.NFCeReservation{}, false, err
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
		UF:           reservationContext.Issuer.AddressState,
		IssuedAt:     issuedAt,
		CNPJ:         reservationContext.Issuer.CNPJ,
		Series:       reservationContext.Config.Series,
		Number:       number,
		NumericCode:  numericCode,
		EmissionType: emissionType,
	})
	if err != nil {
		return fisc.NFCeReservation{}, false, common.ErrValidation
	}

	reservation := fisc.NFCeReservation{
		SaleID:                   saleID,
		Status:                   "reserved",
		Model:                    fisc.NFCeModel,
		Series:                   reservationContext.Config.Series,
		DocumentNumber:           number,
		Environment:              reservationContext.Config.Environment,
		AccessKey:                accessKey,
		EmissionType:             emissionType,
		NumericCode:              numericCode,
		CheckDigit:               int(accessKey[len(accessKey)-1] - '0'),
		IssuedAt:                 issuedAt,
		ContingencyStartedAt:     contingencyStartedAt,
		ContingencyJustification: contingencyJustification,
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
			"sale_id":               saleID,
			"model":                 reservation.Model,
			"series":                reservation.Series,
			"document_number":       reservation.DocumentNumber,
			"environment":           reservation.Environment,
			"emission_type":         reservation.EmissionType,
			"fiscal_snapshot_items": len(snapshots),
		},
	}); err != nil {
		return fisc.NFCeReservation{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeReservation{}, false, err
	}
	return reservation, true, nil
}

func (s *FiscalService) buildSaleItemFiscalSnapshots(
	ctx context.Context,
	tx db.DBTX,
	tenantID, saleID string,
	items []sales.SaleItem,
) ([]fisc.SaleItemFiscalSnapshot, error) {
	if len(items) == 0 {
		return nil, common.ErrFiscalNotReady
	}

	productIDs := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.ID == "" || item.ProductID == "" {
			return nil, common.ErrFiscalNotReady
		}
		if _, ok := seen[item.ProductID]; ok {
			continue
		}
		seen[item.ProductID] = struct{}{}
		productIDs = append(productIDs, item.ProductID)
	}

	products, err := s.products.GetManyByIDs(ctx, tx, tenantID, productIDs)
	if err != nil {
		return nil, err
	}
	profiles, err := s.fiscal.GetProductFiscalProfiles(ctx, tx, tenantID, productIDs)
	if err != nil {
		return nil, err
	}

	snapshots := make([]fisc.SaleItemFiscalSnapshot, 0, len(items))
	for _, item := range items {
		product, ok := products[item.ProductID]
		if !ok || product.NCM == nil || len(strings.TrimSpace(*product.NCM)) != 8 {
			return nil, common.ErrFiscalNotReady
		}
		profile, ok := profiles[item.ProductID]
		if !ok {
			return nil, common.ErrFiscalNotReady
		}

		ncm := strings.TrimSpace(*product.NCM)
		var cest *string
		if product.CEST != nil {
			value := strings.TrimSpace(*product.CEST)
			if value != "" {
				cest = &value
			}
		}

		snapshot := fisc.SaleItemFiscalSnapshot{
			TenantID:             tenantID,
			SaleItemID:           item.ID,
			SaleID:               saleID,
			ProductID:            item.ProductID,
			ProductCode:          strings.TrimSpace(product.SKU),
			ProductDescription:   strings.TrimSpace(product.Name),
			Unit:                 strings.ToUpper(strings.TrimSpace(product.Unit)),
			NCM:                  ncm,
			CEST:                 cest,
			CFOP:                 profile.CFOP,
			ICMSOrigin:           profile.ICMSOrigin,
			ICMSRegime:           profile.ICMSRegime,
			ICMSCode:             profile.ICMSCode,
			PISCST:               profile.PISCST,
			COFINSCST:            profile.COFINSCST,
			IBSCBSCST:            profile.IBSCBSCST,
			IBSCBSClassification: profile.IBSCBSClassification,
			ISCST:                profile.ISCST,
			ISClassification:     profile.ISClassification,
			ReferenceVersion:     profile.ReferenceVersion,
		}
		hash, err := fiscalSnapshotHash(snapshot)
		if err != nil {
			return nil, err
		}
		snapshot.SnapshotSHA256 = hash
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func fiscalSnapshotHash(snapshot fisc.SaleItemFiscalSnapshot) (string, error) {
	// Keep the v1 hash payload stable for snapshots created before migration
	// 0026. Product identity is protected separately by the immutable DB trigger.
	payload := struct {
		TenantID             string  `json:"tenant_id"`
		SaleItemID           string  `json:"sale_item_id"`
		SaleID               string  `json:"sale_id"`
		ProductID            string  `json:"product_id"`
		NCM                  string  `json:"ncm"`
		CEST                 *string `json:"cest,omitempty"`
		CFOP                 string  `json:"cfop"`
		ICMSOrigin           string  `json:"icms_origin"`
		ICMSRegime           string  `json:"icms_regime"`
		ICMSCode             string  `json:"icms_code"`
		PISCST               string  `json:"pis_cst"`
		COFINSCST            string  `json:"cofins_cst"`
		IBSCBSCST            *string `json:"ibs_cbs_cst,omitempty"`
		IBSCBSClassification *string `json:"ibs_cbs_classification,omitempty"`
		ISCST                *string `json:"is_cst,omitempty"`
		ISClassification     *string `json:"is_classification,omitempty"`
		ReferenceVersion     string  `json:"reference_version"`
		SnapshotSHA256       string  `json:"snapshot_sha256"`
	}{
		TenantID:             snapshot.TenantID,
		SaleItemID:           snapshot.SaleItemID,
		SaleID:               snapshot.SaleID,
		ProductID:            snapshot.ProductID,
		NCM:                  snapshot.NCM,
		CEST:                 snapshot.CEST,
		CFOP:                 snapshot.CFOP,
		ICMSOrigin:           snapshot.ICMSOrigin,
		ICMSRegime:           snapshot.ICMSRegime,
		ICMSCode:             snapshot.ICMSCode,
		PISCST:               snapshot.PISCST,
		COFINSCST:            snapshot.COFINSCST,
		IBSCBSCST:            snapshot.IBSCBSCST,
		IBSCBSClassification: snapshot.IBSCBSClassification,
		ISCST:                snapshot.ISCST,
		ISClassification:     snapshot.ISClassification,
		ReferenceVersion:     snapshot.ReferenceVersion,
		SnapshotSHA256:       "",
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func (s *FiscalService) PrepareLegacyOnlyTaxCalculation(
	ctx context.Context,
	tenantID, actorUserID, invoiceID, saleItemID, calculationVersion string,
) (fisc.InvoiceItemTaxCalculation, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	saleItemID = strings.TrimSpace(saleItemID)
	calculationVersion = strings.TrimSpace(calculationVersion)
	if s.validate.Var(invoiceID, "required,uuid") != nil ||
		s.validate.Var(saleItemID, "required,uuid") != nil ||
		calculationVersion == "" ||
		len(calculationVersion) > 120 {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(ctx, tx, tenantID, invoiceID)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if reservation.Status != fisc.NFCeStatusReserved {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrConflict
	}

	snapshots, err := s.fiscal.GetSaleItemFiscalSnapshots(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	var snapshot *fisc.SaleItemFiscalSnapshot
	for i := range snapshots {
		if snapshots[i].SaleItemID == saleItemID {
			snapshot = &snapshots[i]
			break
		}
	}
	if snapshot == nil {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrNotFound
	}
	if snapshot.IBSCBSCST != nil || snapshot.IBSCBSClassification != nil ||
		snapshot.ISCST != nil || snapshot.ISClassification != nil {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrFiscalNotReady
	}

	calculation := fisc.InvoiceItemTaxCalculation{
		TenantID:           tenantID,
		InvoiceID:          reservation.InvoiceID,
		SaleID:             reservation.SaleID,
		SaleItemID:         snapshot.SaleItemID,
		CalculationVersion: calculationVersion,
		LegacyTax: fisc.LegacyTaxCalculation{
			ICMS: fisc.LegacyICMSTax{
				Origin: snapshot.ICMSOrigin,
				Regime: snapshot.ICMSRegime,
				Code:   snapshot.ICMSCode,
			},
			PIS:    fisc.LegacyContributionTax{CST: snapshot.PISCST},
			COFINS: fisc.LegacyContributionTax{CST: snapshot.COFINSCST},
		},
	}
	hash, err := invoiceItemTaxCalculationHash(calculation)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	calculation.CalculationSHA256 = hash

	if err := s.fiscal.InsertInvoiceItemTaxCalculation(ctx, tx, calculation); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID,
		Action:       "fiscal.nfce.tax_calculation.prepare",
		ResourceType: "invoice", ResourceID: reservation.InvoiceID, Outcome: "success",
		Metadata: map[string]any{
			"sale_item_id":        snapshot.SaleItemID,
			"calculation_version": calculation.CalculationVersion,
			"calculation_sha256":  calculation.CalculationSHA256,
			"rtc":                 false,
		},
	}); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	return calculation, nil
}

func fiscalItemNetValues(
	sale sales.Sale,
	items []sales.SaleItem,
) (map[string]platform.Money, error) {
	if len(items) == 0 {
		return nil, common.ErrFiscalNotReady
	}

	var itemDiscountTotal platform.Money
	var allocatableTotal platform.Money
	lineNet := make(map[string]platform.Money, len(items))
	for _, item := range items {
		if item.ID == "" {
			return nil, common.ErrFiscalNotReady
		}
		gross, net, err := item.CalcularSubtotal()
		if err != nil || gross <= 0 || net < 0 {
			return nil, common.ErrFiscalNotReady
		}
		itemDiscountTotal, err = itemDiscountTotal.AddChecked(item.DiscountValue)
		if err != nil {
			return nil, common.ErrValidation
		}
		allocatableTotal, err = allocatableTotal.AddChecked(net)
		if err != nil {
			return nil, common.ErrValidation
		}
		lineNet[item.ID] = net
	}

	globalDiscount, err := sale.DiscountValue.SubChecked(itemDiscountTotal)
	if err != nil || globalDiscount < 0 || globalDiscount > allocatableTotal {
		return nil, common.ErrFiscalNotReady
	}
	if globalDiscount == 0 {
		return lineNet, nil
	}
	if allocatableTotal <= 0 {
		return nil, common.ErrFiscalNotReady
	}

	remainingDiscount := globalDiscount.Cents()
	remainingWeight := allocatableTotal.Cents()
	for index, item := range items {
		net := lineNet[item.ID]
		if index == len(items)-1 {
			netAfter, err := net.SubChecked(platform.NewMoneyCents(remainingDiscount))
			if err != nil || netAfter < 0 {
				return nil, common.ErrFiscalNotReady
			}
			lineNet[item.ID] = netAfter
			break
		}

		share, err := proportionalFloor(remainingDiscount, net.Cents(), remainingWeight)
		if err != nil {
			return nil, err
		}
		netAfter, err := net.SubChecked(platform.NewMoneyCents(share))
		if err != nil || netAfter < 0 {
			return nil, common.ErrFiscalNotReady
		}
		lineNet[item.ID] = netAfter
		remainingDiscount -= share
		remainingWeight -= net.Cents()
	}
	return lineNet, nil
}

func proportionalFloor(value, weight, totalWeight int64) (int64, error) {
	if value < 0 || weight < 0 || totalWeight <= 0 || weight > totalWeight {
		return 0, common.ErrValidation
	}
	product := new(big.Int).Mul(big.NewInt(value), big.NewInt(weight))
	quotient := new(big.Int).Quo(product, big.NewInt(totalWeight))
	if !quotient.IsInt64() {
		return 0, common.ErrValidation
	}
	return quotient.Int64(), nil
}

func (s *FiscalService) PrepareRegularIBSCBSCalculation(
	ctx context.Context,
	tenantID, actorUserID string,
	input PrepareRegularIBSCBSCalculationInput,
) (fisc.InvoiceItemTaxCalculation, error) {
	input.InvoiceID = strings.TrimSpace(input.InvoiceID)
	input.SaleItemID = strings.TrimSpace(input.SaleItemID)
	input.CalculationVersion = strings.TrimSpace(input.CalculationVersion)
	if s.validate.Var(input.InvoiceID, "required,uuid") != nil ||
		s.validate.Var(input.SaleItemID, "required,uuid") != nil ||
		input.CalculationVersion == "" ||
		len(input.CalculationVersion) > 120 {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, input.InvoiceID,
	)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if reservation.Status != fisc.NFCeStatusReserved {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrConflict
	}

	snapshots, err := s.fiscal.GetSaleItemFiscalSnapshots(
		ctx, tx, tenantID, reservation.SaleID,
	)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}

	sale, saleItems, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	netValues, err := fiscalItemNetValues(sale, saleItems)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	expectedBase, ok := netValues[input.SaleItemID]
	if !ok || input.Base != expectedBase {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrValidation
	}
	var snapshot *fisc.SaleItemFiscalSnapshot
	for i := range snapshots {
		if snapshots[i].SaleItemID == input.SaleItemID {
			snapshot = &snapshots[i]
			break
		}
	}
	if snapshot == nil {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrNotFound
	}
	if snapshot.IBSCBSCST == nil || snapshot.IBSCBSClassification == nil {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrFiscalNotReady
	}
	if snapshot.ISCST != nil || snapshot.ISClassification != nil {
		// Selective Tax calculation is deliberately blocked until its current
		// legal calculation table/rules are wired into the engine.
		return fisc.InvoiceItemTaxCalculation{}, common.ErrFiscalNotReady
	}

	rtc, err := fisc.CalculateRegularIBSCBS(fisc.RegularIBSCBSInput{
		CST:            *snapshot.IBSCBSCST,
		Classification: *snapshot.IBSCBSClassification,
		Base:           input.Base,
		IBSUF:          input.IBSUF,
		IBSMunicipal:   input.IBSMunicipal,
		CBS:            input.CBS,
	})
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, common.ErrValidation
	}
	if reservation.IssuedAt.Year() == 2026 {
		if err := fisc.ValidateNFCeReferenceRates2026(rtc); err != nil {
			return fisc.InvoiceItemTaxCalculation{}, common.ErrFiscalNotReady
		}
	}

	calculation := fisc.InvoiceItemTaxCalculation{
		TenantID:           tenantID,
		InvoiceID:          reservation.InvoiceID,
		SaleID:             reservation.SaleID,
		SaleItemID:         snapshot.SaleItemID,
		CalculationVersion: input.CalculationVersion,
		LegacyTax: fisc.LegacyTaxCalculation{
			ICMS: fisc.LegacyICMSTax{
				Origin: snapshot.ICMSOrigin,
				Regime: snapshot.ICMSRegime,
				Code:   snapshot.ICMSCode,
			},
			PIS:    fisc.LegacyContributionTax{CST: snapshot.PISCST},
			COFINS: fisc.LegacyContributionTax{CST: snapshot.COFINSCST},
		},
		RTCTax: fisc.RTCTaxCalculation{IBSCBS: &rtc},
	}
	hash, err := invoiceItemTaxCalculationHash(calculation)
	if err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	calculation.CalculationSHA256 = hash

	if err := s.fiscal.InsertInvoiceItemTaxCalculation(ctx, tx, calculation); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce.tax_calculation.prepare",
		ResourceType: "invoice",
		ResourceID:   reservation.InvoiceID,
		Outcome:      "success",
		Metadata: map[string]any{
			"sale_item_id":        snapshot.SaleItemID,
			"calculation_version": calculation.CalculationVersion,
			"calculation_sha256":  calculation.CalculationSHA256,
		},
	}); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.InvoiceItemTaxCalculation{}, err
	}
	return calculation, nil
}

func invoiceItemTaxCalculationHash(calculation fisc.InvoiceItemTaxCalculation) (string, error) {
	calculation.CalculationSHA256 = ""
	content, err := json.Marshal(calculation)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func validateCalculationMatchesSnapshot(
	snapshot fisc.SaleItemFiscalSnapshot,
	calculation fisc.InvoiceItemTaxCalculation,
) error {
	if calculation.SaleID != snapshot.SaleID ||
		calculation.SaleItemID != snapshot.SaleItemID ||
		calculation.LegacyTax.ICMS.Origin != snapshot.ICMSOrigin ||
		calculation.LegacyTax.ICMS.Regime != snapshot.ICMSRegime ||
		calculation.LegacyTax.ICMS.Code != snapshot.ICMSCode ||
		calculation.LegacyTax.PIS.CST != snapshot.PISCST ||
		calculation.LegacyTax.COFINS.CST != snapshot.COFINSCST {
		return common.ErrFiscalNotReady
	}
	if snapshot.IBSCBSCST != nil || snapshot.IBSCBSClassification != nil {
		if snapshot.IBSCBSCST == nil || snapshot.IBSCBSClassification == nil ||
			calculation.RTCTax.IBSCBS == nil ||
			calculation.RTCTax.IBSCBS.CST != *snapshot.IBSCBSCST ||
			calculation.RTCTax.IBSCBS.Classification != *snapshot.IBSCBSClassification {
			return common.ErrFiscalNotReady
		}
	} else if calculation.RTCTax.IBSCBS != nil {
		return common.ErrFiscalNotReady
	}
	if snapshot.ISCST != nil || snapshot.ISClassification != nil {
		if snapshot.ISCST == nil || snapshot.ISClassification == nil ||
			calculation.RTCTax.IS == nil ||
			calculation.RTCTax.IS.CST != *snapshot.ISCST ||
			calculation.RTCTax.IS.Classification != *snapshot.ISClassification {
			return common.ErrFiscalNotReady
		}
	} else if calculation.RTCTax.IS != nil {
		return common.ErrFiscalNotReady
	}
	hash, err := invoiceItemTaxCalculationHash(calculation)
	if err != nil {
		return err
	}
	if hash != calculation.CalculationSHA256 {
		return common.ErrFiscalNotReady
	}
	return nil
}

func (s *FiscalService) validateInvoiceTaxCalculationsComplete(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	reservation fisc.NFCeReservation,
) error {
	snapshots, err := s.fiscal.GetSaleItemFiscalSnapshots(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return err
	}
	calculations, err := s.fiscal.GetInvoiceItemTaxCalculations(ctx, tx, tenantID, reservation.InvoiceID)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 || len(calculations) != len(snapshots) {
		return common.ErrFiscalNotReady
	}
	byItem := make(map[string]fisc.InvoiceItemTaxCalculation, len(calculations))
	for _, calculation := range calculations {
		byItem[calculation.SaleItemID] = calculation
	}
	for _, snapshot := range snapshots {
		snapshotHash, err := fiscalSnapshotHash(snapshot)
		if err != nil || snapshotHash != snapshot.SnapshotSHA256 {
			return common.ErrFiscalNotReady
		}
		calculation, ok := byItem[snapshot.SaleItemID]
		if !ok {
			return common.ErrFiscalNotReady
		}
		if err := validateCalculationMatchesSnapshot(snapshot, calculation); err != nil {
			return err
		}
	}
	return nil
}

func (s *FiscalService) GetNFCeDocumentDraft(
	ctx context.Context,
	tenantID, invoiceID string,
) (fisc.NFCeDocumentDraft, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if s.validate.Var(invoiceID, "required,uuid") != nil {
		return fisc.NFCeDocumentDraft{}, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(ctx, tx, tenantID, invoiceID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	if reservation.Status != fisc.NFCeStatusReserved {
		return fisc.NFCeDocumentDraft{}, common.ErrConflict
	}
	if err := s.validateInvoiceTaxCalculationsComplete(ctx, tx, tenantID, reservation); err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}

	sale, saleItems, payments, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	if sale.Status != "finalized" {
		return fisc.NFCeDocumentDraft{}, common.ErrSaleNotFinalized
	}
	if err := sale.ValidarPagamentos(payments); err != nil {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}

	reservationContext, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	if reservationContext.Config.Environment != reservation.Environment ||
		reservationContext.Config.Series != reservation.Series {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}

	snapshots, err := s.fiscal.GetSaleItemFiscalSnapshots(ctx, tx, tenantID, reservation.SaleID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	calculations, err := s.fiscal.GetInvoiceItemTaxCalculations(ctx, tx, tenantID, reservation.InvoiceID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	netValues, err := fiscalItemNetValues(sale, saleItems)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}

	snapshotByItem := make(map[string]fisc.SaleItemFiscalSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotByItem[snapshot.SaleItemID] = snapshot
	}
	calculationByItem := make(map[string]fisc.InvoiceItemTaxCalculation, len(calculations))
	for _, calculation := range calculations {
		calculationByItem[calculation.SaleItemID] = calculation
	}

	documentItems := make([]fisc.NFCeDocumentItem, 0, len(saleItems))
	var netTotal platform.Money
	for index, item := range saleItems {
		snapshot, ok := snapshotByItem[item.ID]
		if !ok {
			return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
		}
		calculation, ok := calculationByItem[item.ID]
		if !ok {
			return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
		}
		gross, _, err := item.CalcularSubtotal()
		if err != nil {
			return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
		}
		net, ok := netValues[item.ID]
		if !ok || net < 0 || net > gross {
			return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
		}
		discount, err := gross.SubChecked(net)
		if err != nil || discount < 0 {
			return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
		}
		netTotal, err = netTotal.AddChecked(net)
		if err != nil {
			return fisc.NFCeDocumentDraft{}, err
		}
		documentItems = append(documentItems, fisc.NFCeDocumentItem{
			Number:        index + 1,
			SaleItemID:    item.ID,
			ProductID:     item.ProductID,
			Code:          snapshot.ProductCode,
			Description:   snapshot.ProductDescription,
			Unit:          snapshot.Unit,
			NCM:           snapshot.NCM,
			CEST:          snapshot.CEST,
			CFOP:          snapshot.CFOP,
			Quantity:      item.Qty,
			UnitPrice:     item.UnitPrice,
			GrossValue:    gross,
			DiscountValue: discount,
			NetValue:      net,
			Tax:           calculation,
		})
	}
	if netTotal != sale.Total {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}

	documentPayments := make([]fisc.NFCeDocumentPayment, 0, len(payments))
	for _, payment := range payments {
		documentPayments = append(documentPayments, fisc.NFCeDocumentPayment{
			Method:            payment.Method,
			Amount:            payment.Amount,
			Provider:          payment.Provider,
			TransactionRef:    payment.TransactionRef,
			AuthorizationCode: payment.AuthorizationCode,
			Installments:      payment.Installments,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	return fisc.NFCeDocumentDraft{
		Reservation:     reservation,
		Issuer:          reservationContext.Issuer,
		CustomerID:      sale.CustomerID,
		CommercialTotal: sale.Total,
		Items:           documentItems,
		Payments:        documentPayments,
	}, nil
}

func (s *FiscalService) prepareOfflineQRCodeSignature(
	ctx context.Context,
	tenantID string,
	draft fisc.NFCeDocumentDraft,
) (fisc.NFCeDocumentDraft, error) {
	if draft.Reservation.EmissionType != fisc.NFCeOfflineContingencyEmissionType {
		return draft, nil
	}
	if s.nfceDoc == nil || s.nfceSigner == nil {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}
	cfg, err := s.fiscal.GetNFCeConfig(ctx, tenantID)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	if cfg.CertificateSecretRef == nil ||
		strings.TrimSpace(*cfg.CertificateSecretRef) == "" {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}
	if draft.Reservation.Environment == "production" &&
		(!cfg.Enabled || !s.productionTransmissionStackReady()) {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}
	payload, err := s.nfceDoc.BuildOfflineQRCodeSigningPayload(draft)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	signature, err := s.nfceSigner.SignQRCode(
		ctx,
		strings.TrimSpace(*cfg.CertificateSecretRef),
		payload,
	)
	if err != nil {
		return fisc.NFCeDocumentDraft{}, err
	}
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return fisc.NFCeDocumentDraft{}, common.ErrFiscalNotReady
	}
	draft.QRCodeSignature = &signature
	return draft, nil
}

func (s *FiscalService) BuildNFCeUnsignedCandidate(
	ctx context.Context,
	tenantID, invoiceID string,
) ([]byte, string, error) {
	if s.nfceDoc == nil {
		return nil, "", common.ErrFiscalNotReady
	}
	draft, err := s.GetNFCeDocumentDraft(ctx, tenantID, invoiceID)
	if err != nil {
		return nil, "", err
	}
	draft, err = s.prepareOfflineQRCodeSignature(ctx, tenantID, draft)
	if err != nil {
		return nil, "", err
	}
	content, err := s.nfceDoc.BuildUnsignedLegacyCandidate(draft)
	if err != nil {
		return nil, "", err
	}
	return content, "NFCe-candidate-" + draft.Reservation.AccessKey + ".xml", nil
}

func (s *FiscalService) SignNFCeReserved(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
) (string, string, error) {
	if s.nfceDoc == nil || s.nfceSigner == nil || s.nfceValidator == nil {
		return "", "", common.ErrFiscalNotReady
	}
	draft, err := s.GetNFCeDocumentDraft(ctx, tenantID, invoiceID)
	if err != nil {
		return "", "", err
	}
	draft, err = s.prepareOfflineQRCodeSignature(ctx, tenantID, draft)
	if err != nil {
		return "", "", err
	}
	unsigned, err := s.nfceDoc.BuildUnsignedLegacyCandidate(draft)
	if err != nil {
		return "", "", err
	}
	// The official nfe_v4.00.xsd describes a signed NFe: it requires
	// ds:Signature after infNFeSupl. Validate the *signed* artifact,
	// not the unsigned candidate which cannot satisfy that schema.
	cfg, err := s.fiscal.GetNFCeConfig(ctx, tenantID)
	if err != nil {
		return "", "", err
	}
	if cfg.CertificateSecretRef == nil || strings.TrimSpace(*cfg.CertificateSecretRef) == "" {
		return "", "", common.ErrFiscalNotReady
	}
	if draft.Reservation.Environment == "production" &&
		(!cfg.Enabled || !s.productionTransmissionStackReady()) {
		return "", "", common.ErrFiscalNotReady
	}
	signed, err := s.nfceSigner.Sign(
		ctx,
		strings.TrimSpace(*cfg.CertificateSecretRef),
		draft.Reservation.AccessKey,
		unsigned,
	)
	if err != nil {
		return "", "", err
	}
	if err := s.nfceValidator.Validate(ctx, signed); err != nil {
		return "", "", fmt.Errorf("validate signed NFC-e against configured official XSD: %w", err)
	}
	fileName := "NFCe-" + draft.Reservation.AccessKey + ".xml"
	xmlID, err := s.StoreSignedNFCeXML(
		ctx,
		tenantID,
		actorUserID,
		draft.Reservation.InvoiceID,
		draft.Reservation.AccessKey,
		fileName,
		signed,
	)
	if err != nil {
		return "", "", err
	}
	return xmlID, fileName, nil
}

func (s *FiscalService) RenderNFCeDANFE(
	ctx context.Context,
	tenantID, invoiceID string,
) (string, []byte, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if s.nfceDANFE == nil ||
		s.validate.Var(invoiceID, "required,uuid") != nil {
		return "", nil, common.ErrFiscalNotReady
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		return "", nil, err
	}
	offlinePending := reservation.Status == fisc.NFCeStatusSigned &&
		reservation.EmissionType == fisc.NFCeOfflineContingencyEmissionType
	if reservation.Status != fisc.NFCeStatusAuthorized &&
		reservation.Status != fisc.NFCeStatusCancelled &&
		!offlinePending {
		return "", nil, common.ErrConflict
	}
	if offlinePending {
		if reservation.ContingencyStartedAt == nil ||
			reservation.ContingencyStartedAt.IsZero() ||
			reservation.ContingencyJustification == nil ||
			strings.TrimSpace(*reservation.ContingencyJustification) == "" {
			return "", nil, common.ErrFiscalNotReady
		}
	} else if reservation.AuthorizationProtocol == nil ||
		strings.TrimSpace(*reservation.AuthorizationProtocol) == "" ||
		reservation.AuthorizedAt == nil ||
		reservation.AuthorizedAt.IsZero() {
		return "", nil, common.ErrFiscalNotReady
	}
	_, signedXML, err := s.fiscal.GetLatestNFCeXMLContent(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}

	content, err := s.nfceDANFE.Render(reservation, signedXML)
	if err != nil {
		return "", nil, err
	}
	return "DANFE-NFCe-" + reservation.AccessKey + ".html", content, nil
}

func (s *FiscalService) GetInvoiceTaxCalculations(
	ctx context.Context,
	tenantID, invoiceID string,
) ([]fisc.InvoiceItemTaxCalculation, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if s.validate.Var(invoiceID, "required,uuid") != nil {
		return nil, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(ctx, tx, tenantID, invoiceID)
	if err != nil {
		return nil, err
	}
	calculations, err := s.fiscal.GetInvoiceItemTaxCalculations(
		ctx, tx, tenantID, reservation.InvoiceID,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return calculations, nil
}

func (s *FiscalService) StoreSignedNFCeXML(
	ctx context.Context,
	tenantID, actorUserID, invoiceID, accessKey, fileName string,
	content []byte,
) (string, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	accessKey = strings.TrimSpace(accessKey)
	fileName = strings.TrimSpace(fileName)
	if s.validate.Var(invoiceID, "required,uuid") != nil ||
		fileName == "" ||
		len(content) == 0 ||
		fisc.ValidateNFCeAccessKey(accessKey) != nil {
		return "", common.ErrValidation
	}
	sum := sha256.Sum256(content)
	shaHex := hex.EncodeToString(sum[:])

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		return "", err
	}
	if reservation.Status != fisc.NFCeStatusReserved || reservation.AccessKey != accessKey {
		return "", common.ErrConflict
	}
	if err := s.validateInvoiceTaxCalculationsComplete(ctx, tx, tenantID, reservation); err != nil {
		return "", err
	}

	xmlID, err := s.fiscal.StoreSignedNFCeXML(
		ctx, tx, tenantID, invoiceID, accessKey, fileName, content, shaHex,
	)
	if err != nil {
		return "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce.sign",
		ResourceType: "invoice", ResourceID: invoiceID, Outcome: "success",
		Metadata: map[string]any{
			"access_key":  accessKey,
			"xml_file_id": xmlID,
			"sha256":      shaHex,
		},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return xmlID, nil
}

func (s *FiscalService) MarkNFCeSubmitted(
	ctx context.Context,
	tenantID, actorUserID, invoiceID, accessKey string,
) error {
	invoiceID = strings.TrimSpace(invoiceID)
	accessKey = strings.TrimSpace(accessKey)
	if s.validate.Var(invoiceID, "required,uuid") != nil ||
		fisc.ValidateNFCeAccessKey(accessKey) != nil {
		return common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fiscal.MarkNFCeSubmitted(ctx, tx, tenantID, invoiceID, accessKey); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce.submit",
		ResourceType: "invoice", ResourceID: invoiceID, Outcome: "success",
		Metadata: map[string]any{"access_key": accessKey},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *FiscalService) InutilizeNFCeNumbers(
	ctx context.Context,
	tenantID, actorUserID string,
	req InutilizeNFCeNumbersRequest,
) (fisc.NFCeInutilizationRemoteResult, error) {
	req.Justification = strings.TrimSpace(req.Justification)
	if req.Year < 2006 || req.Year > 2099 ||
		req.Series < 0 || req.Series > 889 ||
		req.StartNumber < 1 || req.EndNumber > 999999999 ||
		req.StartNumber > req.EndNumber ||
		req.EndNumber-req.StartNumber+1 > 10000 ||
		s.validate.Struct(req) != nil {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrValidation
	}
	if s.nfceInutBuilder == nil ||
		s.nfceInutSigner == nil ||
		s.nfceInutValidator == nil ||
		s.nfceInutClient == nil {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrFiscalNotReady
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	contextData, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if !contextData.Config.CertificateReferenceConfigured ||
		contextData.Config.CertificateSecretRef == nil {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrFiscalNotReady
	}
	switch contextData.Config.Environment {
	case "homologation":
	case "production":
		if !contextData.Config.Enabled || !s.productionTransmissionStackReady() {
			return fisc.NFCeInutilizationRemoteResult{}, common.ErrFiscalNotReady
		}
	default:
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrFiscalNotReady
	}

	draft := fisc.NFCeInutilizationDraft{
		Environment:   contextData.Config.Environment,
		IssuerUF:      contextData.Issuer.AddressState,
		IssuerCNPJ:    contextData.Issuer.CNPJ,
		Year:          req.Year,
		Series:        req.Series,
		StartNumber:   req.StartNumber,
		EndNumber:     req.EndNumber,
		Justification: req.Justification,
	}
	unsignedXML, requestID, err := s.nfceInutBuilder.BuildUnsignedInutilization(draft)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrValidation
	}
	if err := s.fiscal.LockNFCeInutilizationRange(
		ctx, tx, tenantID, req.Year, req.Series,
	); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}

	existing, signedXML, existingErr := s.fiscal.GetNFCeInutilizationByRequestForUpdate(
		ctx, tx, tenantID, requestID,
	)
	switch {
	case existingErr == nil:
		if existing.Environment != draft.Environment ||
			existing.Year != draft.Year ||
			existing.Series != draft.Series ||
			existing.StartNumber != draft.StartNumber ||
			existing.EndNumber != draft.EndNumber ||
			existing.Justification != draft.Justification {
			return fisc.NFCeInutilizationRemoteResult{}, common.ErrConflict
		}
		switch existing.Status {
		case fisc.NFCeInutilizationStatusRegistered,
			fisc.NFCeInutilizationStatusRejected,
			fisc.NFCeInutilizationStatusSubmitted:
			if err := tx.Commit(ctx); err != nil {
				return fisc.NFCeInutilizationRemoteResult{}, err
			}
			return inutilizationResultFromRecord(existing), nil
		case fisc.NFCeInutilizationStatusSigned:
			if len(signedXML) == 0 {
				return fisc.NFCeInutilizationRemoteResult{}, common.ErrFiscalNotReady
			}
		default:
			return fisc.NFCeInutilizationRemoteResult{}, common.ErrConflict
		}
	case errors.Is(existingErr, common.ErrNotFound):
		available, err := s.fiscal.NFCeNumberRangeIsAvailable(
			ctx, tx, tenantID, draft.Environment, draft.Year, draft.Series,
			draft.StartNumber, draft.EndNumber,
		)
		if err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
		if !available {
			return fisc.NFCeInutilizationRemoteResult{}, common.ErrConflict
		}

		signedXML, err = s.nfceInutSigner.SignInutilization(
			ctx,
			strings.TrimSpace(*contextData.Config.CertificateSecretRef),
			requestID,
			unsignedXML,
		)
		if err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
		if err := s.nfceInutValidator.Validate(ctx, signedXML); err != nil {
			return fisc.NFCeInutilizationRemoteResult{},
				fmt.Errorf("validate NFC-e inutilization against pinned XSD: %w", err)
		}
		signedHash := sha256.Sum256(signedXML)
		signedSHA := hex.EncodeToString(signedHash[:])
		record := fisc.NFCeInutilization{
			TenantID:      tenantID,
			Environment:   draft.Environment,
			IssuerUF:      strings.ToUpper(strings.TrimSpace(draft.IssuerUF)),
			IssuerCNPJ:    normalizedFiscalCNPJ(draft.IssuerCNPJ),
			Year:          draft.Year,
			Model:         fisc.NFCeModel,
			Series:        draft.Series,
			StartNumber:   draft.StartNumber,
			EndNumber:     draft.EndNumber,
			RequestID:     requestID,
			Status:        fisc.NFCeInutilizationStatusSigned,
			Justification: draft.Justification,
		}
		record.ID, err = s.fiscal.InsertSignedNFCeInutilization(
			ctx, tx, record, actorUserID, signedXML, signedSHA,
		)
		if err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
		if err := s.audit.RecordTx(ctx, tx, audit.Event{
			TenantID:     tenantID,
			ActorUserID:  actorUserID,
			Action:       "fiscal.nfce.inutilization.sign",
			ResourceType: "nfce_number_inutilization",
			ResourceID:   record.ID,
			Outcome:      "success",
			Metadata: map[string]any{
				"request_id":   requestID,
				"environment":  draft.Environment,
				"year":         draft.Year,
				"series":       draft.Series,
				"start_number": draft.StartNumber,
				"end_number":   draft.EndNumber,
			},
		}); err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
	default:
		return fisc.NFCeInutilizationRemoteResult{}, existingErr
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}

	tx, err = s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, lockedXML, err := s.fiscal.GetNFCeInutilizationByRequestForUpdate(
		ctx, tx, tenantID, requestID,
	)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if locked.Status != fisc.NFCeInutilizationStatusSigned || len(lockedXML) == 0 {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrConflict
	}
	if err := s.fiscal.MarkNFCeInutilizationSubmitted(
		ctx, tx, tenantID, requestID,
	); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce.inutilization.submit",
		ResourceType: "nfce_number_inutilization",
		ResourceID:   locked.ID,
		Outcome:      "success",
		Metadata: map[string]any{
			"request_id": requestID,
		},
	}); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	signedXML = lockedXML

	result, err := s.nfceInutClient.Inutilize(
		ctx,
		strings.TrimSpace(*contextData.Config.CertificateSecretRef),
		draft,
		requestID,
		signedXML,
	)
	if err != nil {
		// Keep status=submitted. A replay returns the pending record and never
		// retransmits the same range blindly after an ambiguous network result.
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if result.Pending() &&
		(result.StatusCode != 563 || len(result.ResponseXML) == 0) {
		return result, nil
	}

	responseHash := sha256.Sum256(result.ResponseXML)
	responseSHA := hex.EncodeToString(responseHash[:])

	tx, err = s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, _, err := s.fiscal.GetNFCeInutilizationByRequestForUpdate(
		ctx, tx, tenantID, requestID,
	)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if current.Status != fisc.NFCeInutilizationStatusSubmitted {
		return fisc.NFCeInutilizationRemoteResult{}, common.ErrConflict
	}
	outcome := "rejected"
	if result.Pending() {
		// cStat 563 includes evidence of a previously authorized request.
		// Persist that evidence without changing submitted status; a replay
		// must not send the signed request again.
		if err := s.fiscal.RecordPendingNFCeInutilizationResponse(
			ctx, tx, tenantID, requestID, result, responseSHA,
		); err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
		outcome = "pending_reconciliation"
	} else {
		if err := s.fiscal.ApplyNFCeInutilizationResult(
			ctx, tx, tenantID, requestID, result, responseSHA,
		); err != nil {
			return fisc.NFCeInutilizationRemoteResult{}, err
		}
		if result.Registered() {
			outcome = "registered"
		}
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce.inutilization.result",
		ResourceType: "nfce_number_inutilization",
		ResourceID:   current.ID,
		Outcome:      outcome,
		Metadata: map[string]any{
			"request_id":  requestID,
			"status_code": result.StatusCode,
			"reason":      result.Reason,
		},
	}); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	return result, nil
}

func inutilizationResultFromRecord(
	record fisc.NFCeInutilization,
) fisc.NFCeInutilizationRemoteResult {
	out := fisc.NFCeInutilizationRemoteResult{RequestID: record.RequestID}
	if record.StatusCode != nil {
		out.StatusCode = *record.StatusCode
	}
	if record.Reason != nil {
		out.Reason = *record.Reason
	}
	if record.Protocol != nil {
		out.Protocol = *record.Protocol
	}
	if record.RegisteredAt != nil {
		out.RegisteredAt = *record.RegisteredAt
	}
	switch record.Status {
	case fisc.NFCeInutilizationStatusRegistered:
		out.FinalStatus = fisc.NFCeInutilizationStatusRegistered
	case fisc.NFCeInutilizationStatusRejected:
		out.FinalStatus = fisc.NFCeInutilizationStatusRejected
	}
	return out
}

func normalizedFiscalCNPJ(value string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func (s *FiscalService) CancelNFCe(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
	req CancelNFCeRequest,
) (fisc.NFCeCancellationRemoteResult, error) {
	return s.CancelNFCeHomologation(ctx, tenantID, actorUserID, invoiceID, req)
}

func (s *FiscalService) CancelNFCeHomologation(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
	req CancelNFCeRequest,
) (fisc.NFCeCancellationRemoteResult, error) {
	req.Justification = strings.TrimSpace(req.Justification)
	invoiceID = strings.TrimSpace(invoiceID)
	if s.validate.Var(invoiceID, "required,uuid") != nil ||
		s.validate.Struct(req) != nil {
		return fisc.NFCeCancellationRemoteResult{}, common.ErrValidation
	}
	if s.nfceCancelBuilder == nil ||
		s.nfceCancelSigner == nil ||
		s.nfceEventValidator == nil ||
		s.nfceCancelClient == nil {
		return fisc.NFCeCancellationRemoteResult{}, common.ErrFiscalNotReady
	}

	var (
		reservation fisc.NFCeReservation
		contextData fisc.NFCeReservationContext
		event       fisc.NFCeCancellationEvent
		signedXML   []byte
		secretRef   string
	)

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	reservation, err = s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if reservation.Status != fisc.NFCeStatusAuthorized ||
		reservation.AuthorizationProtocol == nil ||
		strings.TrimSpace(*reservation.AuthorizationProtocol) == "" {
		_ = tx.Rollback(ctx)
		return fisc.NFCeCancellationRemoteResult{}, common.ErrConflict
	}
	contextData, err = s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if reservation.Environment != contextData.Config.Environment ||
		(reservation.Environment != "homologation" && reservation.Environment != "production") ||
		contextData.Config.CertificateSecretRef == nil ||
		strings.TrimSpace(*contextData.Config.CertificateSecretRef) == "" {
		_ = tx.Rollback(ctx)
		return fisc.NFCeCancellationRemoteResult{}, common.ErrFiscalNotReady
	}
	secretRef = strings.TrimSpace(*contextData.Config.CertificateSecretRef)

	existing, existingXML, existingErr := s.fiscal.GetNFCeCancellationEventForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	switch {
	case existingErr == nil:
		event = existing
		signedXML = existingXML
		switch event.Status {
		case fisc.NFCeEventStatusRegistered:
			_ = tx.Rollback(ctx)
			out := fisc.NFCeCancellationRemoteResult{
				AccessKey:   reservation.AccessKey,
				EventID:     event.EventID,
				Sequence:    event.Sequence,
				FinalStatus: fisc.NFCeEventStatusRegistered,
			}
			if event.StatusCode != nil {
				out.StatusCode = *event.StatusCode
			}
			if event.Reason != nil {
				out.Reason = *event.Reason
			}
			if event.Protocol != nil {
				out.Protocol = *event.Protocol
			}
			if event.RegisteredAt != nil {
				out.RegisteredAt = *event.RegisteredAt
			}
			return out, nil
		case fisc.NFCeEventStatusRejected:
			_ = tx.Rollback(ctx)
			out := fisc.NFCeCancellationRemoteResult{
				AccessKey:   reservation.AccessKey,
				EventID:     event.EventID,
				Sequence:    event.Sequence,
				FinalStatus: fisc.NFCeEventStatusRejected,
			}
			if event.StatusCode != nil {
				out.StatusCode = *event.StatusCode
			}
			if event.Reason != nil {
				out.Reason = *event.Reason
			}
			return out, nil
		case fisc.NFCeEventStatusSubmitted:
			_ = tx.Rollback(ctx)
			return fisc.NFCeCancellationRemoteResult{
				AccessKey: reservation.AccessKey,
				EventID:   event.EventID,
				Sequence:  event.Sequence,
			}, nil
		case fisc.NFCeEventStatusSigned:
			if len(signedXML) == 0 {
				_ = tx.Rollback(ctx)
				return fisc.NFCeCancellationRemoteResult{}, common.ErrFiscalNotReady
			}
		default:
			_ = tx.Rollback(ctx)
			return fisc.NFCeCancellationRemoteResult{}, common.ErrConflict
		}
	case errors.Is(existingErr, common.ErrNotFound):
		// First cancellation attempt. Build/sign after releasing the DB lock.
	default:
		_ = tx.Rollback(ctx)
		return fisc.NFCeCancellationRemoteResult{}, existingErr
	}

	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}

	if existingErr != nil {
		draft := fisc.NFCeCancellationDraft{
			Environment:           reservation.Environment,
			IssuerUF:              contextData.Issuer.AddressState,
			IssuerCNPJ:            contextData.Issuer.CNPJ,
			AccessKey:             reservation.AccessKey,
			AuthorizationProtocol: strings.TrimSpace(*reservation.AuthorizationProtocol),
			EventTime:             time.Now(),
			Sequence:              1,
			Justification:         req.Justification,
		}
		unsignedXML, eventID, err := s.nfceCancelBuilder.BuildUnsignedCancellationEvent(draft)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		signedXML, err = s.nfceCancelSigner.SignCancellation(
			ctx, secretRef, eventID, unsignedXML,
		)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		if err := s.nfceEventValidator.Validate(ctx, signedXML); err != nil {
			return fisc.NFCeCancellationRemoteResult{},
				fmt.Errorf("validate NFC-e cancellation event against pinned XSD: %w", err)
		}
		signedHash := sha256.Sum256(signedXML)
		signedSHA := hex.EncodeToString(signedHash[:])

		tx, err = s.uow.Begin(ctx)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		current, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
			ctx, tx, tenantID, invoiceID,
		)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		if current.Status != fisc.NFCeStatusAuthorized ||
			current.AccessKey != reservation.AccessKey {
			return fisc.NFCeCancellationRemoteResult{}, common.ErrConflict
		}
		if _, _, err := s.fiscal.GetNFCeCancellationEventForUpdate(
			ctx, tx, tenantID, invoiceID,
		); err == nil {
			return fisc.NFCeCancellationRemoteResult{}, common.ErrConflict
		} else if !errors.Is(err, common.ErrNotFound) {
			return fisc.NFCeCancellationRemoteResult{}, err
		}

		event = fisc.NFCeCancellationEvent{
			TenantID:      tenantID,
			InvoiceID:     invoiceID,
			EventType:     fisc.NFCeCancellationEventType,
			Sequence:      1,
			EventID:       eventID,
			Environment:   reservation.Environment,
			Status:        fisc.NFCeEventStatusSigned,
			Justification: req.Justification,
		}
		event.ID, err = s.fiscal.InsertSignedNFCeCancellationEvent(
			ctx, tx, event, actorUserID, signedXML, signedSHA,
		)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		if err := s.audit.RecordTx(ctx, tx, audit.Event{
			TenantID:     tenantID,
			ActorUserID:  actorUserID,
			Action:       "fiscal.nfce.cancel.sign",
			ResourceType: "invoice_fiscal_event",
			ResourceID:   event.ID,
			Outcome:      "success",
			Metadata: map[string]any{
				"invoice_id": invoiceID,
				"event_id":   event.EventID,
				"access_key": reservation.AccessKey,
			},
		}); err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
	}

	tx, err = s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockedEvent, lockedXML, err := s.fiscal.GetNFCeCancellationEventForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if lockedEvent.EventID != event.EventID ||
		lockedEvent.Status != fisc.NFCeEventStatusSigned ||
		len(lockedXML) == 0 {
		return fisc.NFCeCancellationRemoteResult{}, common.ErrConflict
	}
	if err := s.fiscal.MarkNFCeCancellationSubmitted(
		ctx, tx, tenantID, event.EventID,
	); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce.cancel.submit",
		ResourceType: "invoice_fiscal_event",
		ResourceID:   lockedEvent.ID,
		Outcome:      "success",
		Metadata: map[string]any{
			"invoice_id": invoiceID,
			"event_id":   event.EventID,
			"access_key": reservation.AccessKey,
		},
	}); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	signedXML = lockedXML

	result, err := s.nfceCancelClient.Cancel(
		ctx,
		secretRef,
		contextData.Issuer.AddressState,
		reservation.Environment,
		reservation.AccessKey,
		reservation.DocumentNumber,
		event.EventID,
		event.Sequence,
		signedXML,
	)
	if err != nil {
		// Keep status=submitted. Never retransmit blindly after an ambiguous call.
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if result.Pending() {
		return result, nil
	}

	responseHash := sha256.Sum256(result.ResponseXML)
	responseSHA := hex.EncodeToString(responseHash[:])

	tx, err = s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, invoiceID,
	); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if err := s.fiscal.ApplyNFCeCancellationResult(
		ctx, tx, tenantID, invoiceID, event.EventID, result, responseSHA,
	); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	outcome := "rejected"
	if result.Registered() {
		outcome = "registered"
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce.cancel.result",
		ResourceType: "invoice_fiscal_event",
		ResourceID:   event.ID,
		Outcome:      outcome,
		Metadata: map[string]any{
			"invoice_id":  invoiceID,
			"event_id":    event.EventID,
			"access_key":  reservation.AccessKey,
			"status_code": result.StatusCode,
			"reason":      result.Reason,
		},
	}); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	return result, nil
}

func (s *FiscalService) AuthorizeNFCe(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
) (fisc.NFCeRemoteOutcome, error) {
	return s.AuthorizeNFCeHomologation(ctx, tenantID, actorUserID, invoiceID)
}

func (s *FiscalService) AuthorizeNFCeHomologation(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
) (fisc.NFCeRemoteOutcome, error) {
	if s.nfceAuthorizer == nil {
		return fisc.NFCeRemoteOutcome{}, common.ErrFiscalNotReady
	}
	invoiceID = strings.TrimSpace(invoiceID)
	if s.validate.Var(invoiceID, "required,uuid") != nil {
		return fisc.NFCeRemoteOutcome{}, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	reservation, err := s.fiscal.GetNFCeReservationByInvoiceForUpdate(
		ctx, tx, tenantID, invoiceID,
	)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	reservationContext, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	if reservation.Environment != reservationContext.Config.Environment ||
		(reservation.Environment != "homologation" && reservation.Environment != "production") ||
		reservationContext.Config.CertificateSecretRef == nil ||
		strings.TrimSpace(*reservationContext.Config.CertificateSecretRef) == "" {
		return fisc.NFCeRemoteOutcome{}, common.ErrFiscalNotReady
	}

	secretRef := strings.TrimSpace(*reservationContext.Config.CertificateSecretRef)
	var (
		signedXML   []byte
		doAuthorize bool
	)
	switch reservation.Status {
	case fisc.NFCeStatusSigned:
		if reservation.Environment == "production" &&
			!reservationContext.Config.Enabled &&
			reservation.EmissionType != fisc.NFCeOfflineContingencyEmissionType {
			return fisc.NFCeRemoteOutcome{}, common.ErrFiscalNotReady
		}
		_, signedXML, err = s.fiscal.GetLatestNFCeXMLContent(
			ctx, tx, tenantID, reservation.InvoiceID,
		)
		if err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
		if err := s.fiscal.MarkNFCeSubmitted(
			ctx, tx, tenantID, reservation.InvoiceID, reservation.AccessKey,
		); err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
		if err := s.audit.RecordTx(ctx, tx, audit.Event{
			TenantID:     tenantID,
			ActorUserID:  actorUserID,
			Action:       "fiscal.nfce.submit",
			ResourceType: "invoice",
			ResourceID:   reservation.InvoiceID,
			Outcome:      "success",
			Metadata: map[string]any{
				"access_key": reservation.AccessKey,
				"mode":       "authorize",
			},
		}); err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
		doAuthorize = true
	case fisc.NFCeStatusSubmitted:
		// Ambiguous/retry state: consult the access key. Never resend blindly.
	default:
		return fisc.NFCeRemoteOutcome{}, common.ErrConflict
	}

	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}

	var outcome fisc.NFCeRemoteOutcome
	if doAuthorize {
		outcome, err = s.nfceAuthorizer.Authorize(
			ctx,
			secretRef,
			reservationContext.Issuer.AddressState,
			reservation.Environment,
			reservation.AccessKey,
			reservation.DocumentNumber,
			signedXML,
		)
	} else {
		outcome, err = s.nfceAuthorizer.Consult(
			ctx,
			secretRef,
			reservationContext.Issuer.AddressState,
			reservation.Environment,
			reservation.AccessKey,
		)
	}
	if err != nil {
		// Keep status=submitted. A later call will consult by access key.
		return fisc.NFCeRemoteOutcome{}, err
	}
	if outcome.Pending() {
		return outcome, nil
	}

	result := fisc.NFCeAuthorizationResult{
		AccessKey: reservation.AccessKey,
	}
	if outcome.Authorized() {
		result.Status = fisc.NFCeStatusAuthorized
		result.Protocol = outcome.Protocol
		result.ProtocolXML = append([]byte(nil),outcome.ProtocolXML...)
		result.AuthorizedAt = outcome.ReceivedAt
	} else if outcome.Rejected() {
		result.Status = fisc.NFCeStatusRejected
		result.RejectionCode = fmt.Sprintf("%d", outcome.StatusCode)
		result.RejectionMessage = outcome.Reason
	} else {
		return outcome, nil
	}
	if err := s.ApplyNFCeAuthorizationResult(
		ctx, tenantID, actorUserID, reservation.InvoiceID, result,
	); err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	return outcome, nil
}

func (s *FiscalService) ApplyNFCeAuthorizationResult(
	ctx context.Context,
	tenantID, actorUserID, invoiceID string,
	result fisc.NFCeAuthorizationResult,
) error {
	invoiceID = strings.TrimSpace(invoiceID)
	result.AccessKey = strings.TrimSpace(result.AccessKey)
	result.Protocol = strings.TrimSpace(result.Protocol)
	result.RejectionCode = strings.TrimSpace(result.RejectionCode)
	result.RejectionMessage = strings.TrimSpace(result.RejectionMessage)

	if s.validate.Var(invoiceID, "required,uuid") != nil ||
		fisc.ValidateNFCeAccessKey(result.AccessKey) != nil {
		return common.ErrValidation
	}
	switch {
	case result.IsAuthorized():
		if result.Protocol == "" || result.AuthorizedAt.IsZero() ||
			result.RejectionCode != "" || result.RejectionMessage != "" {
			return common.ErrValidation
		}
	case result.IsRejected():
		if result.RejectionCode == "" || result.RejectionMessage == "" ||
			result.Protocol != "" || !result.AuthorizedAt.IsZero() {
			return common.ErrValidation
		}
	default:
		return common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var processed []byte
	var processedSHA string
	if result.IsAuthorized() {
		// No update to authorized is committed without the real SEFAZ
		// protocol being bound to our signed XML and archived as nfeProc.
		// An ambiguous response remains submitted and is consulted again.
		if s.nfceProcessed == nil || len(result.ProtocolXML)==0 {
			return common.ErrFiscalNotReady
		}
		_, signedXML, err := s.fiscal.GetLatestNFCeXMLContent(ctx, tx, tenantID, invoiceID)
		if err != nil { return err }
		processed, err = s.nfceProcessed.Build(
			signedXML, result.ProtocolXML, result.AccessKey, result.Protocol, result.AuthorizedAt,
		)
		if err != nil { return fmt.Errorf("SEFAZ protocol / signed NFC-e mismatch: %w",err) }
		sum := sha256.Sum256(processed)
		processedSHA = hex.EncodeToString(sum[:])
	}
	if err := s.fiscal.ApplyNFCeAuthorizationResult(ctx, tx, tenantID, invoiceID, result); err != nil {
		return err
	}
	if result.IsAuthorized() {
		if err := s.fiscal.StoreAuthorizedNFCeProcessedXML(
			ctx,tx,tenantID,invoiceID,result.AccessKey,
			result.AccessKey+"-procNFe.xml",result.ProtocolXML,processed,processedSHA,
		); err != nil { return err }
	}
	action := "fiscal.nfce.rejected"
	metadata := map[string]any{
		"access_key":     result.AccessKey,
		"rejection_code": result.RejectionCode,
	}
	if result.IsAuthorized() {
		action = "fiscal.nfce.authorized"
		metadata = map[string]any{
			"access_key":             result.AccessKey,
			"authorization_protocol": result.Protocol,
		}
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: action,
		ResourceType: "invoice", ResourceID: invoiceID, Outcome: "success",
		Metadata: metadata,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *FiscalService) GetProductFiscalProfile(
	ctx context.Context,
	tenantID, productID string,
) (fisc.ProductFiscalProfile, error) {
	productID = strings.TrimSpace(productID)
	if s.validate.Var(productID, "required,uuid") != nil {
		return fisc.ProductFiscalProfile{}, common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profiles, err := s.fiscal.GetProductFiscalProfiles(ctx, tx, tenantID, []string{productID})
	if err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	profile, ok := profiles[productID]
	if !ok {
		return fisc.ProductFiscalProfile{}, common.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	return profile, nil
}

func (s *FiscalService) PrepareProductFiscalProfile(
	ctx context.Context,
	tenantID, actorUserID, productID string,
	req PrepareProductFiscalProfileRequest,
) (fisc.ProductFiscalProfile, error) {
	productID = strings.TrimSpace(productID)
	req.CFOP = strings.TrimSpace(req.CFOP)
	req.ICMSOrigin = strings.TrimSpace(req.ICMSOrigin)
	req.ICMSRegime = strings.ToLower(strings.TrimSpace(req.ICMSRegime))
	req.ICMSCode = strings.TrimSpace(req.ICMSCode)
	req.PISCST = strings.TrimSpace(req.PISCST)
	req.COFINSCST = strings.TrimSpace(req.COFINSCST)
	req.ReferenceVersion = strings.TrimSpace(req.ReferenceVersion)
	req.IBSCBSCST = normalizedOptionalString(req.IBSCBSCST)
	req.IBSCBSClassification = normalizedOptionalString(req.IBSCBSClassification)
	req.ISCST = normalizedOptionalString(req.ISCST)
	req.ISClassification = normalizedOptionalString(req.ISClassification)

	if s.validate.Var(productID, "required,uuid") != nil || s.validate.Struct(req) != nil {
		return fisc.ProductFiscalProfile{}, common.ErrValidation
	}
	if req.ICMSOrigin < "0" || req.ICMSOrigin > "8" {
		return fisc.ProductFiscalProfile{}, common.ErrValidation
	}
	switch req.ICMSRegime {
	case "cst":
		if len(req.ICMSCode) != 2 {
			return fisc.ProductFiscalProfile{}, common.ErrValidation
		}
	case "csosn":
		if len(req.ICMSCode) != 3 {
			return fisc.ProductFiscalProfile{}, common.ErrValidation
		}
	default:
		return fisc.ProductFiscalProfile{}, common.ErrValidation
	}
	if !pairedFiscalCodes(req.IBSCBSCST, req.IBSCBSClassification, true) ||
		!pairedFiscalCodes(req.ISCST, req.ISClassification, false) {
		return fisc.ProductFiscalProfile{}, common.ErrValidation
	}

	profile := fisc.ProductFiscalProfile{
		TenantID:             tenantID,
		ProductID:            productID,
		CFOP:                 req.CFOP,
		ICMSOrigin:           req.ICMSOrigin,
		ICMSRegime:           req.ICMSRegime,
		ICMSCode:             req.ICMSCode,
		PISCST:               req.PISCST,
		COFINSCST:            req.COFINSCST,
		IBSCBSCST:            req.IBSCBSCST,
		IBSCBSClassification: req.IBSCBSClassification,
		ISCST:                req.ISCST,
		ISClassification:     req.ISClassification,
		ReferenceVersion:     req.ReferenceVersion,
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fiscal.UpsertProductFiscalProfile(ctx, tx, tenantID, actorUserID, profile); err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.product_profile.prepare",
		ResourceType: "product",
		ResourceID:   productID,
		Outcome:      "success",
		Metadata: map[string]any{
			"cfop":              profile.CFOP,
			"icms_regime":       profile.ICMSRegime,
			"icms_code":         profile.ICMSCode,
			"pis_cst":           profile.PISCST,
			"cofins_cst":        profile.COFINSCST,
			"reference_version": profile.ReferenceVersion,
		},
	}); err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.ProductFiscalProfile{}, err
	}
	return profile, nil
}

func normalizedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func pairedFiscalCodes(cst, classification *string, requirePrefix bool) bool {
	if cst == nil && classification == nil {
		return true
	}
	if cst == nil || classification == nil {
		return false
	}
	if requirePrefix && !strings.HasPrefix(*classification, *cst) {
		return false
	}
	return true
}

func productionNFCeDataReady(readiness fisc.NFCeReadiness) bool {
	if !readiness.ConfigExists ||
		readiness.Environment != "production" ||
		!readiness.IssuerIdentityConfigured ||
		!readiness.IssuerAddressConfigured ||
		!readiness.MunicipalityCodeConfigured ||
		!readiness.CertificateReferenceConfigured ||
		readiness.ActiveProducts < 1 ||
		readiness.ProductsMissingNCM > 0 ||
		readiness.ProductsMissingFiscalProfile > 0 {
		return false
	}
	for _, reason := range readiness.BlockingReasons {
		if reason != "homologation_environment" {
			return false
		}
	}
	return true
}

func (s *FiscalService) productionTransmissionStackReady() bool {
	return s.nfceDoc != nil &&
		s.nfceSigner != nil &&
		s.nfceValidator != nil &&
		s.nfceAuthorizer != nil &&
		s.nfceCancelBuilder != nil &&
		s.nfceCancelSigner != nil &&
		s.nfceEventValidator != nil &&
		s.nfceCancelClient != nil &&
		s.nfceInutBuilder != nil &&
		s.nfceInutSigner != nil &&
		s.nfceInutValidator != nil &&
		s.nfceInutClient != nil
}

func (s *FiscalService) SetNFCeProductionTransmission(
	ctx context.Context,
	tenantID, actorUserID string,
	enabled bool,
) (fisc.NFCeConfig, error) {
	cfg, err := s.fiscal.GetNFCeConfig(ctx, tenantID)
	if err != nil {
		return fisc.NFCeConfig{}, err
	}
	if enabled {
		if cfg.Environment != "production" ||
			!cfg.CertificateReferenceConfigured ||
			!s.productionTransmissionStackReady() {
			return fisc.NFCeConfig{}, common.ErrFiscalNotReady
		}
		readiness, err := s.fiscal.GetNFCeReadiness(ctx, tenantID)
		if err != nil {
			return fisc.NFCeConfig{}, err
		}
		if !productionNFCeDataReady(readiness) {
			return fisc.NFCeConfig{}, common.ErrFiscalNotReady
		}
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return fisc.NFCeConfig{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fiscal.SetNFCeTransmissionEnabled(
		ctx, tx, tenantID, actorUserID, enabled,
	); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID:     tenantID,
		ActorUserID:  actorUserID,
		Action:       "fiscal.nfce_config.transmission",
		ResourceType: "nfce_config",
		ResourceID:   tenantID,
		Outcome:      "success",
		Metadata: map[string]any{
			"environment": cfg.Environment,
			"enabled":     enabled,
		},
	}); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeConfig{}, err
	}
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
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

	// The issuer is part of the NFC-e identity. Never change it while a
	// reserved, signed or submitted document could still need its XML built
	// or its remote status reconciled.
	if err := s.fiscal.LockNFCeTenantForUpdate(ctx, tx, tenantID); err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	hasOpenWork, err := s.fiscal.HasOpenNFCeWork(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	if hasOpenWork {
		return fisc.NFCeIssuerProfile{}, common.ErrConflict
	}

	if err := s.fiscal.UpdateNFCeIssuerProfile(ctx, tx, tenantID, profile); err != nil {
		return fisc.NFCeIssuerProfile{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce_issuer.prepare",
		ResourceType: "company", ResourceID: tenantID, Outcome: "success",
		Metadata: map[string]any{
			"crt":       profile.CRT,
			"state":     profile.AddressState,
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

	if err := s.fiscal.LockNFCeTenantForUpdate(ctx, tx, tenantID); err != nil {
		return fisc.NFCeConfig{}, err
	}
	// The certificate reference is write-only in public API responses.
	// Leaving the field blank on a later edit must preserve the existing
	// server-side reference, never wipe it or disclose it to the client.
	if req.CertificateSecretRef == "" {
		current, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
		if errors.Is(err, common.ErrNotFound) {
			return fisc.NFCeConfig{}, common.ErrValidation
		}
		if err != nil {
			return fisc.NFCeConfig{}, err
		}
		if current.Config.CertificateSecretRef == nil ||
			strings.TrimSpace(*current.Config.CertificateSecretRef) == "" {
			return fisc.NFCeConfig{}, common.ErrValidation
		}
		cfg.CertificateSecretRef = current.Config.CertificateSecretRef
	}
	hasOpenWork, err := s.fiscal.HasOpenNFCeWork(ctx, tx, tenantID)
	if err != nil {
		return fisc.NFCeConfig{}, err
	}
	if hasOpenWork {
		// Certificate/CSC references may be rotated to recover transport,
		// but switching environment or series would strand in-flight
		// access keys and make a follow-up consultation unsafe.
		current, err := s.fiscal.GetNFCeReservationContextForUpdate(ctx, tx, tenantID)
		if err != nil {
			return fisc.NFCeConfig{}, err
		}
		if current.Config.Environment != cfg.Environment ||
			current.Config.Series != cfg.Series {
			return fisc.NFCeConfig{}, common.ErrConflict
		}
	}

	if err := s.fiscal.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, cfg); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "fiscal.nfce_config.prepare",
		ResourceType: "nfce_config", ResourceID: tenantID, Outcome: "success",
		Metadata: map[string]any{
			"environment": cfg.Environment,
			"series":      cfg.Series,
			"enabled":     false,
		},
	}); err != nil {
		return fisc.NFCeConfig{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeConfig{}, err
	}
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
}
