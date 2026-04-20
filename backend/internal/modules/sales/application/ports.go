package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type SalesRepository interface {
	InsertSale(ctx context.Context, tx db.DBTX, s sales.Sale) (string, error)
	InsertItem(ctx context.Context, tx db.DBTX, it sales.SaleItem) error
	InsertPayment(ctx context.Context, tx db.DBTX, p sales.Payment) error
	GetSale(ctx context.Context, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
	ListSales(ctx context.Context, limit, offset int) ([]sales.Sale, int, error)
	CancelSale(ctx context.Context, tx db.DBTX, id string, reason string) error
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type CashRepository interface {
	EnsureDefaultRegister(ctx context.Context) (string, error)
	OpenSession(ctx context.Context, tx db.DBTX, registerID, userID string, openingAmount float64, notes *string) (string, error)
	CloseSession(ctx context.Context, tx db.DBTX, sessionID, userID string, closingAmount float64, notes *string) error
	GetSession(ctx context.Context, tx db.DBTX, sessionID string) (sales.CashSession, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, productID string, qty float64) error
	InsertMovement(ctx context.Context, tx db.DBTX, m inv.InventoryMovement) error
}

type ProductsRepository interface {
	GetManyByIDs(ctx context.Context, tx db.DBTX, ids []string) (map[string]inv.Product, error)
}

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, e fin.LedgerEntry, createdByUserID *string) (string, error)
}
