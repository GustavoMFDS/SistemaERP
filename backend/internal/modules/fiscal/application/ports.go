package application

import (
	"context"
	"time"

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
	SignQRCode(ctx context.Context, secretRef string, payload string) (string, error)
}

type NFCeSchemaValidator interface {
	Validate(ctx context.Context, unsignedXML []byte) error
}

type NFCeRemoteAuthorizer interface {
	Authorize(
		ctx context.Context,
		certificateSecretRef string,
		issuerUF string,
		environment string,
		accessKey string,
		documentNumber int64,
		signedXML []byte,
	) (fisc.NFCeRemoteOutcome, error)
	Consult(
		ctx context.Context,
		certificateSecretRef string,
		issuerUF string,
		environment string,
		accessKey string,
	) (fisc.NFCeRemoteOutcome, error)
}

type NFCeDocumentBuilder interface {
	BuildUnsignedLegacyCandidate(draft fisc.NFCeDocumentDraft) ([]byte, error)
	BuildOfflineQRCodeSigningPayload(draft fisc.NFCeDocumentDraft) (string, error)
}

type NFCeDANFERenderer interface {
	Render(reservation fisc.NFCeReservation, signedXML []byte) ([]byte, error)
}

type NFCeCancellationEventBuilder interface {
	BuildUnsignedCancellationEvent(draft fisc.NFCeCancellationDraft) ([]byte, string, error)
}

type NFCeCancellationEventSigner interface {
	SignCancellation(
		ctx context.Context,
		secretRef string,
		expectedEventID string,
		unsignedXML []byte,
	) ([]byte, error)
}

type NFCeRemoteCancellationClient interface {
	Cancel(
		ctx context.Context,
		certificateSecretRef string,
		issuerUF string,
		environment string,
		accessKey string,
		documentNumber int64,
		eventID string,
		sequence int,
		signedEventXML []byte,
	) (fisc.NFCeCancellationRemoteResult, error)
}

type NFCeInutilizationBuilder interface {
	BuildUnsignedInutilization(draft fisc.NFCeInutilizationDraft) ([]byte, string, error)
}

type NFCeInutilizationSigner interface {
	SignInutilization(
		ctx context.Context,
		secretRef string,
		expectedRequestID string,
		unsignedXML []byte,
	) ([]byte, error)
}

type NFCeRemoteInutilizationClient interface {
	Inutilize(
		ctx context.Context,
		certificateSecretRef string,
		draft fisc.NFCeInutilizationDraft,
		requestID string,
		signedXML []byte,
	) (fisc.NFCeInutilizationRemoteResult, error)
}

// NFCeProcessedDocumentBuilder must only accept an original signed XML
// and the exact protocol XML captured from a SEFAZ response.
type NFCeProcessedDocumentBuilder interface {
 Build(signedXML, protocolXML []byte, accessKey, protocol string, authorizedAt time.Time) ([]byte,error)
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
	LockNFCeTenantForUpdate(ctx context.Context, tx db.DBTX, tenantID string) error
	HasOpenNFCeWork(ctx context.Context, tx db.DBTX, tenantID string) (bool, error)
	GetNFCeConfig(ctx context.Context, tenantID string) (fisc.NFCeConfig, error)
	UpsertNFCeConfig(ctx context.Context, tx db.DBTX, tenantID string, actorUserID string, cfg fisc.NFCeConfig) error
	SetNFCeTransmissionEnabled(ctx context.Context, tx db.DBTX, tenantID, actorUserID string, enabled bool) error
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
	StoreAuthorizedNFCeProcessedXML(ctx context.Context, tx db.DBTX, tenantID, invoiceID, accessKey, fileName string, protocolXML, processedXML []byte, sha256 string) error
	GetAuthorizedNFCeProcessedXML(ctx context.Context, tenantID, invoiceID string) (fileName string, xml []byte, sha256 string, err error)
	GetProductFiscalProfiles(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) (map[string]fisc.ProductFiscalProfile, error)
	UpsertProductFiscalProfile(ctx context.Context, tx db.DBTX, tenantID, actorUserID string, profile fisc.ProductFiscalProfile) error
	CreateSaleItemFiscalSnapshots(ctx context.Context, tx db.DBTX, tenantID string, snapshots []fisc.SaleItemFiscalSnapshot) error
	GetSaleItemFiscalSnapshots(ctx context.Context, tx db.DBTX, tenantID, saleID string) ([]fisc.SaleItemFiscalSnapshot, error)
	InsertInvoiceItemTaxCalculation(ctx context.Context, tx db.DBTX, calculation fisc.InvoiceItemTaxCalculation) error
	GetInvoiceItemTaxCalculations(ctx context.Context, tx db.DBTX, tenantID, invoiceID string) ([]fisc.InvoiceItemTaxCalculation, error)
	GetLatestNFCeXMLContent(ctx context.Context, tx db.DBTX, tenantID, invoiceID string) (fileName string, content []byte, err error)
	GetNFCeCancellationEventForUpdate(ctx context.Context, tx db.DBTX, tenantID, invoiceID string) (fisc.NFCeCancellationEvent, []byte, error)
	InsertSignedNFCeCancellationEvent(ctx context.Context, tx db.DBTX, event fisc.NFCeCancellationEvent, actorUserID string, signedXML []byte, sha256 string) (string, error)
	MarkNFCeCancellationSubmitted(ctx context.Context, tx db.DBTX, tenantID, eventID string) error
	ApplyNFCeCancellationResult(ctx context.Context, tx db.DBTX, tenantID, invoiceID, eventID string, result fisc.NFCeCancellationRemoteResult, responseSHA256 string) error
	LockNFCeInutilizationRange(ctx context.Context, tx db.DBTX, tenantID string, year, series int) error
	NFCeNumberRangeIsAvailable(ctx context.Context, tx db.DBTX, tenantID, environment string, year, series int, startNumber, endNumber int64) (bool, error)
	GetNFCeInutilizationByRequestForUpdate(ctx context.Context, tx db.DBTX, tenantID, requestID string) (fisc.NFCeInutilization, []byte, error)
	InsertSignedNFCeInutilization(ctx context.Context, tx db.DBTX, record fisc.NFCeInutilization, actorUserID string, signedXML []byte, sha256 string) (string, error)
	MarkNFCeInutilizationSubmitted(ctx context.Context, tx db.DBTX, tenantID, requestID string) error
	ApplyNFCeInutilizationResult(ctx context.Context, tx db.DBTX, tenantID, requestID string, result fisc.NFCeInutilizationRemoteResult, responseSHA256 string) error
	RecordPendingNFCeInutilizationResponse(ctx context.Context, tx db.DBTX, tenantID, requestID string, result fisc.NFCeInutilizationRemoteResult, responseSHA256 string) error
}

type SalesRepository interface {
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}
