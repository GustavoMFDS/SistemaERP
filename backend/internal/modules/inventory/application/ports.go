package application

import (
	"context"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type ProductsRepository interface {
	List(ctx context.Context, query string, limit, offset int) ([]inv.Product, int, error)
	Get(ctx context.Context, id string) (inv.Product, error)
	Create(ctx context.Context, tx db.DBTX, p inv.Product) (string, error)
	Update(ctx context.Context, tx db.DBTX, id string, p inv.Product) error
	GetManyByIDs(ctx context.Context, tx db.DBTX, ids []string) (map[string]inv.Product, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, productID string, qty float64) error
	InsertMovement(ctx context.Context, tx db.DBTX, m inv.InventoryMovement) error
	LowStock(ctx context.Context, limit int) ([]inv.Product, error)
	ListMovements(ctx context.Context, productID string, limit, offset int) ([]inv.InventoryMovement, int, error)
}
