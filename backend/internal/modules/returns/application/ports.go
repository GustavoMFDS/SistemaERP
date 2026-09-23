package application

import (
	"context"

	"github.com/example/sistemaemgo/internal/modules/returns/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type Repository interface {
	List(ctx context.Context, tenantID, saleID string, limit, offset int) ([]domain.SaleReturn, int, error)
	Get(ctx context.Context, tenantID, id string) (domain.SaleReturn, []domain.Item, error)
	GetSaleForUpdate(ctx context.Context, tx db.DBTX, tenantID, saleID string) (sales.Sale, []sales.SaleItem, error)
	SumReturnedBySaleItem(ctx context.Context, tx db.DBTX, tenantID string, saleItemIDs []string) (map[string]platform.Quantity, error)
	CreateReturn(ctx context.Context, tx db.DBTX, tenantID string, r domain.SaleReturn) (string, error)
	InsertItem(ctx context.Context, tx db.DBTX, tenantID string, item domain.Item) error

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (returnID, requestHash string, refundDue platform.Money, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, returnID string, refundDue platform.Money) error
}
