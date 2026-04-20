package application

import (
	"context"

	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
)

type FinanceRepository interface {
	InsertLedgerEntry(ctx context.Context, tx db.DBTX, e fin.LedgerEntry, createdByUserID *string) (string, error)
	Dashboard(ctx context.Context, from, to string) (map[string]float64, error)
	ListLedger(ctx context.Context, limit, offset int) ([]fin.LedgerEntry, int, error)
}
