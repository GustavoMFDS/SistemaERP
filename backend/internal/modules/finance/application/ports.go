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
	ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error)
}
