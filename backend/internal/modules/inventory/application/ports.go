package application

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type ProductsRepository interface {
	List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error)
	Get(ctx context.Context, tenantID string, id string) (inv.Product, error)
	Create(ctx context.Context, tx db.DBTX, tenantID string, p inv.Product) (string, error)
	Update(ctx context.Context, tx db.DBTX, tenantID string, id string, p inv.Product) error
	GetManyByIDs(ctx context.Context, tx db.DBTX, tenantID string, ids []string) (map[string]inv.Product, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID string, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID string, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, tenantID string, productID string, qty float64) error
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, m inv.InventoryMovement) error
	LowStock(ctx context.Context, tenantID string, limit int) ([]inv.Product, error)
	ListMovements(ctx context.Context, tenantID string, productID string, limit, offset int) ([]inv.InventoryMovement, int, error)
}
