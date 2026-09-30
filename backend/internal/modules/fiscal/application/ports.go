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
	ExistsInvoiceForSale(ctx context.Context, tx db.DBTX, tenantID string, saleID string) (bool, error)
	CreateInvoiceWithXML(ctx context.Context, tx db.DBTX, tenantID string, saleID, companyID string, createdByUserID *string, fileName string, content []byte, sha256 string) (invoiceID, xmlID string, err error)
	ListXML(ctx context.Context, tenantID string, limit, offset int) ([]fisc.XMLFile, int, error)
	GetXMLContent(ctx context.Context, tenantID string, id string) (fileName string, content []byte, err error)
	GetNFCeReadiness(ctx context.Context, tenantID string) (fisc.NFCeReadiness, error)
}

type SalesRepository interface {
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}
