package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type Repository interface {
	ListReturns(ctx context.Context, tenantID, saleID string, limit, offset int) ([]ret.Return, int, error)
	GetReturn(ctx context.Context, tenantID, returnID string) (ret.Return, []ret.ReturnItem, []ret.Refund, error)
	GetReturnForUpdate(ctx context.Context, tx db.DBTX, tenantID, returnID string) (ret.Return, error)
	GetRefund(ctx context.Context, tenantID, refundID string) (ret.Refund, error)
	GetReturnedAggregates(ctx context.Context, tx db.DBTX, tenantID, saleID string) (map[string]ret.ReturnedAggregate, error)
	CreateReturn(ctx context.Context, tx db.DBTX, tenantID string, value ret.Return) (string, error)
	InsertReturnItem(ctx context.Context, tx db.DBTX, tenantID string, item ret.ReturnItem) error
	CreateRefund(ctx context.Context, tx db.DBTX, tenantID string, value ret.Refund) (string, error)

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (resourceID, resultStatus, requestHash string, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, resourceID, resultStatus string) error
}

type SalesRepository interface {
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error)
}

type InventoryRepository interface {
	EnsureBalanceRow(ctx context.Context, tx db.DBTX, tenantID, productID string) error
	GetBalanceForUpdate(ctx context.Context, tx db.DBTX, tenantID, productID string) (inv.InventoryBalance, error)
	UpdateBalance(ctx context.Context, tx db.DBTX, tenantID, productID string, qty platform.Quantity) error
	InsertMovement(ctx context.Context, tx db.DBTX, tenantID string, movement inv.InventoryMovement) error
}

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, tenantID string, entry fin.LedgerEntry, createdByUserID *string) (string, error)
}
