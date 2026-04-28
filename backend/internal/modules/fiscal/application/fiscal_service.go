package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

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

func (s *FiscalService) GenerateNFeXML(ctx context.Context, actorUserID string, req GenerateXMLRequest) (invoiceID, xmlID string, err error) {
	if err := s.validate.Struct(req); err != nil {
		return "", "", common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	exists, err := s.fiscal.ExistsInvoiceForSale(ctx, tx, req.SaleID)
	if err != nil {
		return "", "", err
	}
	if exists {
		return "", "", common.ErrInvoiceAlreadyExists
	}

	sale, items, _, err := s.sales.GetSale(ctx, req.SaleID)
	if err != nil {
		return "", "", common.ErrNotFound
	}
	if sale.Status != "finalized" {
		return "", "", common.ErrSaleNotFinalized
	}

	companyID, err := s.fiscal.GetCompanyID(ctx, tx)
	if err != nil {
		return "", "", err
	}
	if s.nfe == nil {
		return "", "", fmt.Errorf("nfe provider not configured")
	}

	prodIDs := uniqueProductIDs(items)
	prodMap, err := s.products.GetManyByIDs(ctx, tx, prodIDs)
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
	invID, xmlFileID, err := s.fiscal.CreateInvoiceWithXML(ctx, tx, req.SaleID, companyID, &actor, fileName, xmlBytes, shaHex)
	if err != nil {
		return "", "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return invID, xmlFileID, nil
}

func (s *FiscalService) ListXML(ctx context.Context, limit, offset int) ([]fisc.XMLFile, int, error) {
	return s.fiscal.ListXML(ctx, limit, offset)
}

func (s *FiscalService) DownloadXML(ctx context.Context, id string) (string, []byte, error) {
	return s.fiscal.GetXMLContent(ctx, id)
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
