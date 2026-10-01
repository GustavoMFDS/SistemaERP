package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
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

type PrepareProductFiscalProfileRequest struct {
	CFOP                  string  `json:"cfop" validate:"required,numeric,len=4"`
	ICMSOrigin            string  `json:"icms_origin" validate:"required,numeric,len=1"`
	ICMSRegime            string  `json:"icms_regime" validate:"required,oneof=cst csosn"`
	ICMSCode              string  `json:"icms_code" validate:"required,numeric"`
	PISCST                 string  `json:"pis_cst" validate:"required,numeric,len=2"`
	COFINSCST              string  `json:"cofins_cst" validate:"required,numeric,len=2"`
	IBSCBSCST              *string `json:"ibs_cbs_cst" validate:"omitempty,numeric,len=3"`
	IBSCBSClassification   *string `json:"ibs_cbs_classification" validate:"omitempty,numeric,len=6"`
	ISCST                  *string `json:"is_cst" validate:"omitempty,numeric,len=3"`
	ISClassification       *string `json:"is_classification" validate:"omitempty,numeric,len=6"`
	ReferenceVersion       string  `json:"reference_version" validate:"required,min=2,max=120"`
}

type PrepareRegularIBSCBSCalculationInput struct {
	InvoiceID          string                         `json:"invoice_id"`
	SaleItemID         string                         `json:"sale_item_id"`
	CalculationVersion string                         `json:"calculation_version"`
	Base               platform.Money                 `json:"base"`
	IBSUF              fisc.RegularTaxComponentInput  `json:"ibs_uf"`
	IBSMunicipal       fisc.RegularTaxComponentInput  `json:"ibs_municipal"`
	CBS                fisc.RegularTaxComponentInput  `json:"cbs"`
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

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, saleID)
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
		!reservationContext.Config.CertificateReferenceConfigured {
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
			TenantID: tenantID,
			SaleItemID: item.ID,
			SaleID: saleID,
			ProductID: item.ProductID,
			NCM: ncm,
			CEST: cest,
			CFOP: profile.CFOP,
			ICMSOrigin: profile.ICMSOrigin,
			ICMSRegime: profile.ICMSRegime,
			ICMSCode: profile.ICMSCode,
			PISCST: profile.PISCST,
			COFINSCST: profile.COFINSCST,
			IBSCBSCST: profile.IBSCBSCST,
			IBSCBSClassification: profile.IBSCBSClassification,
			ISCST: profile.ISCST,
			ISClassification: profile.ISClassification,
			ReferenceVersion: profile.ReferenceVersion,
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
	snapshot.SnapshotSHA256 = ""
	content, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
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
			"access_key": accessKey,
			"xml_file_id": xmlID,
			"sha256": shaHex,
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

	if err := s.fiscal.ApplyNFCeAuthorizationResult(ctx, tx, tenantID, invoiceID, result); err != nil {
		return err
	}
	action := "fiscal.nfce.rejected"
	metadata := map[string]any{
		"access_key": result.AccessKey,
		"rejection_code": result.RejectionCode,
	}
	if result.IsAuthorized() {
		action = "fiscal.nfce.authorized"
		metadata = map[string]any{
			"access_key": result.AccessKey,
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
		TenantID: tenantID,
		ProductID: productID,
		CFOP: req.CFOP,
		ICMSOrigin: req.ICMSOrigin,
		ICMSRegime: req.ICMSRegime,
		ICMSCode: req.ICMSCode,
		PISCST: req.PISCST,
		COFINSCST: req.COFINSCST,
		IBSCBSCST: req.IBSCBSCST,
		IBSCBSClassification: req.IBSCBSClassification,
		ISCST: req.ISCST,
		ISClassification: req.ISClassification,
		ReferenceVersion: req.ReferenceVersion,
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
		TenantID: tenantID,
		ActorUserID: actorUserID,
		Action: "fiscal.product_profile.prepare",
		ResourceType: "product",
		ResourceID: productID,
		Outcome: "success",
		Metadata: map[string]any{
			"cfop": profile.CFOP,
			"icms_regime": profile.ICMSRegime,
			"icms_code": profile.ICMSCode,
			"pis_cst": profile.PISCST,
			"cofins_cst": profile.COFINSCST,
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
