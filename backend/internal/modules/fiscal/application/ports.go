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
type NFCeXMLSigner interface {
	Sign(ctx context.Context, secretRef string, expectedAccessKey string, unsignedXML []byte) ([]byte, error)
}

type NFCeDocumentBuilder interface {
	BuildUnsignedLegacyCandidate(draft fisc.NFCeDocumentDraft) ([]byte, error)
}

type NFeProvider interface {
	GenerateNFeXML(ctx context.Context, sale sales.Sale, items []sales.SaleItem, products map[string]inv.Product) (content []byte, fileName string, err error)
}

type FiscalRepository interface {
	ExistsInvoiceForSale(ctx context.Context, tx db.DBTX, tenantID string, saleID string) (bool, error)
	CreateInvoiceWithXML(ctx context.Context, tx db.DBTX, tenantID string, saleID, companyID string, createdByUserID *string, fileName string, content []byte, sha256 string) (invoiceID, xmlID string, err error)
	ListXML(ctx context.Context, tenantID string, limit, offset int) ([]fisc.XMLFile, int, error)
	GetXMLContent(ctx context.Context, tenantID string, id string) (fileName string, content []byte, err error)
	GetNFCeReadiness(ctx context.Context, tenantID string) (fisc.NFCeReadiness, error)
	GetNFCeConfig(ctx context.Context, tenantID string) (fisc.NFCeConfig, error)
	UpsertNFCeConfig(ctx context.Context, tx db.DBTX, tenantID string, actorUserID string, cfg fisc.NFCeConfig) error
	GetNFCeIssuerProfile(ctx context.Context, tenantID string) (fisc.NFCeIssuerProfile, error)
	UpdateNFCeIssuerProfile(ctx context.Context, tx db.DBTX, tenantID string, profile fisc.NFCeIssuerProfile) error
	GetNFCeReservationContextForUpdate(ctx context.Context, tx db.DBTX, tenantID string) (fisc.NFCeReservationContext, error)
	GetNFCeReservationBySale(ctx context.Context, tx db.DBTX, tenantID, saleID string) (fisc.NFCeReservation, error)
	GetNFCeReservationByInvoiceForUpdate(ctx context.Context, tx db.DBTX, tenantID, invoiceID string) (fisc.NFCeReservation, error)
	CreateNFCeReservation(ctx context.Context, tx db.DBTX, tenantID, actorUserID string, reservation fisc.NFCeReservation) (string, error)
	ReserveNextNFCeNumber(ctx context.Context, tx db.DBTX, tenantID string, series int) (int64, error)
	StoreSignedNFCeXML(ctx context.Context, tx db.DBTX, tenantID, invoiceID, accessKey, fileName string, content []byte, sha256 string) (string, error)
	MarkNFCeSubmitted(ctx context.Context, tx db.DBTX, tenantID, invoiceID, accessKey string) error
	ApplyNFCeAuthorizationResult(ctx context.Context, tx db.DBTX, tenantID, invoiceID string, result fisc.NFCeAuthorizationResult) error
	GetProductFiscalProfiles(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) (map[string]fisc.ProductFiscalProfile, error)
	UpsertProductFiscalProfile(ctx context.Context, tx db.DBTX, tenantID, actorUserID string, profile fisc.ProductFiscalProfile) error
	CreateSaleItemFiscalSnapshots(ctx context.Context, tx db.DBTX, tenantID string, snapshots []fisc.SaleItemFiscalSnapshot) error
	GetSaleItemFiscalSnapshots(ctx context.Context, tx db.DBTX, tenantID, saleID string) ([]fisc.SaleItemFiscalSnapshot, error)
	InsertInvoiceItemTaxCalculation(ctx context.Context, tx db.DBTX, calculation fisc.InvoiceItemTaxCalculation) error
	GetInvoiceItemTaxCalculations(ctx context.Context, tx db.DBTX, tenantID, invoiceID string) ([]fisc.InvoiceItemTaxCalculation, error)
}

type SalesRepository interface {
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}
