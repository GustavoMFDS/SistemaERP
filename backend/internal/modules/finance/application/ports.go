package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, tenantID string, e fin.LedgerEntry, createdByUserID *string) (string, error)
	Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]platform.Money, error)
	OwnerOverview(ctx context.Context, tenantID, from, to string) (fin.OwnerOverview, error)
	ProductRanking(ctx context.Context, tenantID, from, to string, limit int) ([]fin.ProductRankingRow, error)
	ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error)

	ListPayments(ctx context.Context, tenantID, from, to, method, status string, limit, offset int) ([]fin.PaymentRecord, int, error)
	GetPaymentForUpdate(ctx context.Context, tx db.DBTX, tenantID, paymentID string) (fin.PaymentRecord, error)
	CreatePaymentReconciliation(ctx context.Context, tx db.DBTX, tenantID string, item fin.PaymentReconciliation) (string, error)
	GetPaymentReconciliation(ctx context.Context, tenantID, paymentID string) (fin.PaymentReconciliation, error)
	ListPaymentReconciliationAdjustments(ctx context.Context, tenantID, paymentID string) ([]fin.PaymentReconciliationAdjustment, error)
	CreatePaymentReconciliationAdjustment(ctx context.Context, tx db.DBTX, tenantID string, item fin.PaymentReconciliationAdjustment) (string, error)
	UpdatePaymentReconciliation(ctx context.Context, tx db.DBTX, tenantID, paymentID, status string, received, fee platform.Money, notes *string, actorUserID string) error

	ListReturnRefunds(ctx context.Context, tenantID, status string, limit, offset int) ([]fin.ReturnRefundSummary, int, error)
	GetReturnForUpdate(ctx context.Context, tx db.DBTX, tenantID, returnID string) (fin.ReturnRefundSummary, error)
	SumReturnRefunds(ctx context.Context, tx db.DBTX, tenantID, returnID string) (platform.Money, error)
	CreateReturnRefund(ctx context.Context, tx db.DBTX, tenantID string, item fin.ReturnRefund) (string, error)

	GetOpenCashAvailable(ctx context.Context, tx db.DBTX, tenantID, cashSessionID string) (platform.Money, error)
	InsertCashWithdrawal(ctx context.Context, tx db.DBTX, tenantID, cashSessionID, actorUserID string, amount platform.Money, notes *string) (string, error)

	LockIdempotencyKey(ctx context.Context, tx db.DBTX, tenantID, operation, key string) error
	GetIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key string) (resourceID, resultStatus, requestHash string, resultAmount *platform.Money, ok bool, err error)
	SaveIdempotencyResult(ctx context.Context, tx db.DBTX, tenantID, operation, key, requestHash, resourceID, resultStatus string, resultAmount *platform.Money) error
}
