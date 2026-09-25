package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type SalesRepository interface {
	CustomerBelongsToTenant(ctx context.Context, tx db.DBTX, tenantID, customerID string) (bool, error)
	InsertSale(ctx context.Context, tx db.DBTX, tenantID string, s sales.Sale) (string, error)
	InsertItem(ctx context.Context, tx db.DBTX, tenantID string, it sales.SaleItem) error
	InsertPayment(ctx context.Context, tx db.DBTX, tenantID string, p sales.Payment) error
	GetSale(ctx context.Context, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
	ListSales(ctx context.Context, tenantID string, limit, offset int) ([]sales.Sale, int, error)
	CancelSale(ctx context.Context, tx db.DBTX, tenantID string, id string, reason string) error
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
	HasInvoiceForSale(ctx context.Context, tx db.DBTX, tenantID string, id string) (bool, error)

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (saleID string, total platform.Money, requestHash string, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, saleID string, total platform.Money) error
}

type CashRepository interface {
	EnsureDefaultRegister(ctx context.Context, tenantID string) (string, error)
	OpenSession(ctx context.Context, tx db.DBTX, tenantID string, registerID, userID string, openingAmount platform.Money, notes *string) (string, error)
	GetOpenSession(ctx context.Context, tenantID string, registerID string) (sales.CashSession, bool, error)
	CloseSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID, userID string, expectedCash, closingAmount platform.Money, notes *string) error
	GetSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID string) (sales.CashSession, error)
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID, sessionID, userID, movementType string, amount platform.Money, notes *string) (string, error)
	SumPaymentsByMethod(ctx context.Context, tx db.DBTX, tenantID, sessionID string) (map[string]platform.Money, error)
	SumMovements(ctx context.Context, tx db.DBTX, tenantID, sessionID string) (supply platform.Money, withdrawal platform.Money, err error)
	SaveReconciliation(ctx context.Context, tx db.DBTX, tenantID, sessionID string, expected, declared map[string]platform.Money) error
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID string, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, tenantID string, productID string, qty platform.Quantity) error
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, m inv.InventoryMovement) error
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, tenantID string, e fin.LedgerEntry, createdByUserID *string) (string, error)
}
