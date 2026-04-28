package application

import (
	"context"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

// NFeProvider encapsulates NF-e XML generation.
//
// The current implementation is an MVP/stub (well-formed XML) and is NOT SEFAZ-ready.
// This abstraction exists so we can later add:
// - digital signature
// - SEFAZ transmission + protocol handling
// - DANFE generation
// without changing the service/API surface.
type NFeProvider interface {
	GenerateNFeXML(ctx context.Context, sale sales.Sale, items []sales.SaleItem, products map[string]inv.Product) (content []byte, fileName string, err error)
}

type FiscalRepository interface {
	ExistsInvoiceForSale(ctx context.Context, tx db.DBTX, saleID string) (bool, error)
	GetCompanyID(ctx context.Context, tx db.DBTX) (string, error)
	CreateInvoiceWithXML(ctx context.Context, tx db.DBTX, saleID, companyID string, createdByUserID *string, fileName string, content []byte, sha256 string) (invoiceID, xmlID string, err error)
	ListXML(ctx context.Context, limit, offset int) ([]fisc.XMLFile, int, error)
	GetXMLContent(ctx context.Context, id string) (fileName string, content []byte, err error)
}

type SalesRepository interface {
	GetSale(ctx context.Context, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, ids []string) (map[string]inv.Product, error)
}
