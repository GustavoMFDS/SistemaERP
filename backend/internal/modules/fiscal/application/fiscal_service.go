package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

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
	validate *validator.Validate
	logger   *slog.Logger
}

type GenerateXMLRequest struct {
	SaleID string `json:"sale_id" validate:"required"`
}

type PrepareNFCeConfigRequest struct {
	Environment          string `json:"environment" validate:"required,oneof=homologation production"`
	Series               int    `json:"series" validate:"min=0,max=889"`
	CSCID                string `json:"csc_id" validate:"required,max=32"`
	CSCSecretRef         string `json:"csc_secret_ref" validate:"required,max=500"`
	CertificateSecretRef string `json:"certificate_secret_ref" validate:"required,max=500"`
}

func NewFiscalService(uow db.UnitOfWork, fiscal FiscalRepository, salesRepo SalesRepository, productsRepo ProductsRepository, v *validator.Validate, logger *slog.Logger) *FiscalService {
	return &FiscalService{uow: uow, fiscal: fiscal, sales: salesRepo, products: productsRepo, nfe: nil, validate: v, logger: logger}
}

func NewFiscalServiceWithProvider(
	uow db.UnitOfWork,
	fiscal FiscalRepository,
	salesRepo SalesRepository,
	productsRepo ProductsRepository,
	nfeProvider NFeProvider,
	v *validator.Validate,
	logger *slog.Logger,
) *FiscalService {
	return &FiscalService{uow: uow, fiscal: fiscal, sales: salesRepo, products: productsRepo, nfe: nfeProvider, validate: v, logger: logger}
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
	return s.fiscal.GetNFCeReadiness(ctx, tenantID)
}

func (s *FiscalService) GetNFCeConfig(ctx context.Context, tenantID string) (fisc.NFCeConfig, error) {
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
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

	cscID := req.CSCID
	cscSecretRef := req.CSCSecretRef
	certificateSecretRef := req.CertificateSecretRef
	cfg := fisc.NFCeConfig{
		TenantID:             tenantID,
		Enabled:              false,
		Environment:          req.Environment,
		Series:               req.Series,
		CSCID:                &cscID,
		CSCSecretRef:         &cscSecretRef,
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
	if err := tx.Commit(ctx); err != nil {
		return fisc.NFCeConfig{}, err
	}
	return s.fiscal.GetNFCeConfig(ctx, tenantID)
}
