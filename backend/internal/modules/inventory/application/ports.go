package application

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type ProductsRepository interface {
	List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error)
	Get(ctx context.Context, tenantID string, id string) (inv.Product, error)
	GetByBarcode(ctx context.Context, tenantID string, barcode string) (inv.Product, error)
	Create(ctx context.Context, tx db.DBTX, tenantID string, p inv.Product) (string, error)
	LockProductImportKey(ctx context.Context, tx db.DBTX, tenantID, key string) error
	GetProductImportBatch(ctx context.Context, tx db.DBTX, tenantID, key string) (batchID, requestHash string, itemCount int, found bool, err error)
	LookupProductImportBatch(ctx context.Context, tenantID, key string) (batchID string, itemCount int, found bool, err error)
	CreateProductImportBatch(ctx context.Context, tx db.DBTX, tenantID, actorID, key, requestHash string, itemCount int) (string, error)
	Update(ctx context.Context, tx db.DBTX, tenantID string, id string, p inv.Product, preserveCost bool) error
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
	GetManyBySKUs(ctx context.Context, tx db.DBTX, tenantID string, skus []string) (map[string]inv.Product, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID string, productID string) error
	EnsureBalanceRows(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) error
	GetBalancesForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) (map[string]inv.InventoryBalance, error)
	LockOpeningStockKey(ctx context.Context, tx db.DBTX, tenantID, key string) error
	GetOpeningStockBatch(ctx context.Context, tx db.DBTX, tenantID, key string) (batchID, requestHash string, itemCount int, found bool, err error)
	LookupOpeningStockBatch(ctx context.Context, tenantID, key string) (batchID string, itemCount int, found bool, err error)
	CreateOpeningStockBatch(ctx context.Context, tx db.DBTX, tenantID, actorID, key, requestHash string, itemCount int) (string, error)
	HasAnyStockMovements(ctx context.Context, tx db.DBTX, tenantID string, productIDs []string) (bool, error)
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, tenantID string, productID string, qty platform.Quantity) error
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, m inv.InventoryMovement) error
	LowStock(ctx context.Context, tenantID string, limit int) ([]inv.Product, error)
	LowStockCount(ctx context.Context, tenantID string) (int, error)
	ListMovements(ctx context.Context, tenantID string, productID string, limit, offset int) ([]inv.InventoryMovement, int, error)
}
