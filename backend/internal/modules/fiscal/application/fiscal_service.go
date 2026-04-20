package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FiscalService struct {
	fiscal   FiscalRepository
	sales    SalesRepository
	products ProductsRepository
	validate *validator.Validate
	logger   *slog.Logger
}

type GenerateXMLRequest struct {
	SaleID string `json:"sale_id" validate:"required"`
}

func NewFiscalService(fiscal FiscalRepository, salesRepo SalesRepository, productsRepo ProductsRepository, v *validator.Validate, logger *slog.Logger) *FiscalService {
	return &FiscalService{fiscal: fiscal, sales: salesRepo, products: productsRepo, validate: v, logger: logger}
}

func (s *FiscalService) GenerateNFeXML(ctx context.Context, pool *pgxpool.Pool, actorUserID string, req GenerateXMLRequest) (invoiceID, xmlID string, err error) {
	if err := s.validate.Struct(req); err != nil {
		return "", "", common.ErrValidation
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
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

	xmlBytes, fileName, err := buildNFeXML(ctx, s.products, tx, sale, items)
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

// buildNFeXML is an MVP generator: produces a well-formed, internally consistent XML.
// It is NOT a SEFAZ-ready NF-e.
func buildNFeXML(ctx context.Context, products ProductsRepository, tx db.DBTX, sale sales.Sale, items []sales.SaleItem) ([]byte, string, error) {
	// Minimal structure inspired by NF-e concepts.
	type InfAdic struct {
		InfCpl string `xml:"infCpl"`
	}
	type ICMSTot struct {
		VProd string `xml:"vProd"`
		VDesc string `xml:"vDesc"`
		VNF   string `xml:"vNF"`
	}
	type Total struct {
		ICMSTot ICMSTot `xml:"ICMSTot"`
	}
	type Ide struct {
		NatOp  string `xml:"natOp"`
		Mod    string `xml:"mod"`
		Serie  string `xml:"serie"`
		NNF    string `xml:"nNF"`
		DhEmi  string `xml:"dhEmi"`
		TpNF   string `xml:"tpNF"`
		FinNFe string `xml:"finNFe"`
	}
	type Prod struct {
		CProd  string `xml:"cProd"`
		XProd  string `xml:"xProd"`
		CEAN   string `xml:"cEAN"`
		UCom   string `xml:"uCom"`
		QCom   string `xml:"qCom"`
		VUnCom string `xml:"vUnCom"`
		VProd  string `xml:"vProd"`
		NCM    string `xml:"NCM"`
		CFOP   string `xml:"CFOP"`
	}
	type Det struct {
		NItem string `xml:"nItem,attr"`
		Prod  Prod   `xml:"prod"`
	}
	type InfNFe struct {
		ID      string   `xml:"Id,attr"`
		Versao  string   `xml:"versao,attr"`
		Ide     Ide      `xml:"ide"`
		Det     []Det    `xml:"det"`
		Total   Total    `xml:"total"`
		InfAdic *InfAdic `xml:"infAdic,omitempty"`
	}
	type NFe struct {
		XMLName xml.Name `xml:"NFe"`
		InfNFe  InfNFe   `xml:"infNFe"`
	}

	// Load product snapshots
	prodIDs := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		if !seen[it.ProductID] {
			seen[it.ProductID] = true
			prodIDs = append(prodIDs, it.ProductID)
		}
	}
	prodMap, err := products.GetManyByIDs(ctx, tx, prodIDs)
	if err != nil {
		return nil, "", err
	}

	now := time.Now().Format(time.RFC3339)
	nfe := NFe{InfNFe: InfNFe{
		ID:     "NFe" + sale.ID,
		Versao: "4.00",
		Ide: Ide{
			NatOp:  "VENDA",
			Mod:    "55",
			Serie:  "1",
			NNF:    "1",
			DhEmi:  now,
			TpNF:   "1",
			FinNFe: "1",
		},
		InfAdic: &InfAdic{InfCpl: fmt.Sprintf("XML MVP gerado a partir da venda %s", sale.ID)},
	}}

	var vProd float64
	for i, it := range items {
		p := prodMap[it.ProductID]
		lineGross := it.UnitPrice * it.Qty
		vProd += lineGross
		barcode := "SEM GTIN"
		if p.Barcode != nil {
			barcode = *p.Barcode
		}
		// Defaults (configuráveis no futuro)
		ncm := "00000000"
		cfop := "5102"
		nfe.InfNFe.Det = append(nfe.InfNFe.Det, Det{
			NItem: fmt.Sprintf("%d", i+1),
			Prod: Prod{
				CProd:  p.SKU,
				XProd:  p.Name,
				CEAN:   barcode,
				UCom:   p.Unit,
				QCom:   fmt.Sprintf("%.3f", it.Qty),
				VUnCom: fmt.Sprintf("%.2f", it.UnitPrice),
				VProd:  fmt.Sprintf("%.2f", lineGross),
				NCM:    ncm,
				CFOP:   cfop,
			},
		})
	}

	nfe.InfNFe.Total = Total{ICMSTot: ICMSTot{
		VProd: fmt.Sprintf("%.2f", vProd),
		VDesc: fmt.Sprintf("%.2f", sale.DiscountValue),
		VNF:   fmt.Sprintf("%.2f", sale.Total),
	}}

	out, err := xml.MarshalIndent(nfe, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append([]byte(xml.Header), out...)
	fileName := fmt.Sprintf("NFe-%s.xml", sale.ID)
	return out, fileName, nil
}
