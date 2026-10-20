package application

import (
	"context"

	proc "github.com/example/sistemaemgo/internal/modules/procurement/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type Repository interface {
	ListSuppliers(ctx context.Context, tenantID, query string, limit, offset int) ([]proc.Supplier, int, error)
	GetSupplier(ctx context.Context, q db.DBTX, tenantID, id string) (proc.Supplier, error)
	CreateSupplier(ctx context.Context, tx db.DBTX, tenantID string, supplier proc.Supplier) (string, error)
	UpdateSupplier(ctx context.Context, tx db.DBTX, tenantID, id string, supplier proc.Supplier) error

	ListPurchases(ctx context.Context, tenantID, status string, limit, offset int) ([]proc.Purchase, int, error)
	GetPurchase(ctx context.Context, tenantID, id string) (proc.Purchase, []proc.PurchaseItem, []proc.Receipt, error)
	GetPurchaseForUpdate(ctx context.Context, tx db.DBTX, tenantID, id string) (proc.Purchase, []proc.PurchaseItem, error)
	CreatePurchase(ctx context.Context, tx db.DBTX, tenantID string, purchase proc.Purchase, items []proc.PurchaseItem) (string, error)
	CreateReceipt(ctx context.Context, tx db.DBTX, tenantID, purchaseID, actorUserID string, notes *string) (string, error)
	InsertReceiptItem(ctx context.Context, tx db.DBTX, tenantID string, item proc.ReceiptItem) error
	UpdateItemReceived(ctx context.Context, tx db.DBTX, tenantID, itemID string, qtyReceived platform.Quantity) error
	UpdatePurchaseStatus(ctx context.Context, tx db.DBTX, tenantID, purchaseID string, status proc.PurchaseStatus, receivedAt *string) error
	CancelPurchase(ctx context.Context, tx db.DBTX, tenantID, purchaseID string) error
	UpdateProductCost(ctx context.Context, tx db.DBTX, tenantID, productID string, cost platform.Money) error
	CreateAccountPayable(ctx context.Context, tx db.DBTX, tenantID, purchaseID, supplierID string, description string, amount platform.Money, dueDate string) error
	CancelAccountPayable(ctx context.Context, tx db.DBTX, tenantID, purchaseID string) error

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (resourceID, resultStatus, requestHash string, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, resourceID, resultStatus string) error
}
