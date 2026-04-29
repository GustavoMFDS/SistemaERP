package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type SalesRepository interface {
	InsertSale(ctx context.Context, tx db.DBTX, tenantID string, s sales.Sale) (string, error)
	InsertItem(ctx context.Context, tx db.DBTX, tenantID string, it sales.SaleItem) error
	InsertPayment(ctx context.Context, tx db.DBTX, tenantID string, p sales.Payment) error
	GetSale(ctx context.Context, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
	ListSales(ctx context.Context, tenantID string, limit, offset int) ([]sales.Sale, int, error)
	CancelSale(ctx context.Context, tx db.DBTX, tenantID string, id string, reason string) error
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (saleID string, total float64, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, saleID string, total float64) error
}

type CashRepository interface {
	EnsureDefaultRegister(ctx context.Context, tenantID string) (string, error)
	OpenSession(ctx context.Context, tx db.DBTX, tenantID string, registerID, userID string, openingAmount float64, notes *string) (string, error)
	CloseSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID, userID string, closingAmount float64, notes *string) error
	GetSession(ctx context.Context, tx db.DBTX, tenantID string, sessionID string) (sales.CashSession, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID string, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, tenantID string, productID string, qty float64) error
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, m inv.InventoryMovement) error
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, tenantID string, e fin.LedgerEntry, createdByUserID *string) (string, error)
}
