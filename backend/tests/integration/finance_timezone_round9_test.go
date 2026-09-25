//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFinanceDashboardUsesBusinessLocalDayBoundaries(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var tenantID string
	if err := pool.QueryRow(ctx, `
		SELECT id::text
		FROM companies
		ORDER BY created_at
		LIMIT 1
	`).Scan(&tenantID); err != nil {
		t.Fatalf("seeded tenant required: %v", err)
	}

	entryType := fmt.Sprintf("round9_timezone_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM ledger_entries
			WHERE tenant_id=$1 AND entry_type=$2
		`, tenantID, entryType)
	})

	fixtures := []struct {
		at     string
		amount string
	}{
		// America/Sao_Paulo is UTC-03 on these dates.
		{"2026-09-25T02:59:59Z", "1.00"}, // 2026-09-24 23:59:59 local: exclude
		{"2026-09-25T03:00:00Z", "2.00"}, // 2026-09-25 00:00:00 local: include
		{"2026-09-26T02:59:59Z", "3.00"}, // 2026-09-25 23:59:59 local: include
		{"2026-09-26T03:00:00Z", "4.00"}, // 2026-09-26 00:00:00 local: exclude
	}

	for _, fixture := range fixtures {
		if _, err := pool.Exec(ctx, `
			INSERT INTO ledger_entries(
				tenant_id, entry_type, amount_gross, amount_discount,
				amount_net, profit_estimated, notes, created_at
			)
			VALUES ($1,$2,$3,0,$3,0,'round9 timezone boundary',$4::timestamptz)
		`, tenantID, entryType, fixture.amount, fixture.at); err != nil {
			t.Fatalf("insert finance boundary fixture %s: %v", fixture.at, err)
		}
	}

	repo := fininfra.NewFinanceRepo(pool)
	totals, err := repo.Dashboard(
		ctx,
		tenantID,
		"2026-09-25",
		"2026-09-25",
		"America/Sao_Paulo",
	)
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}

	got, ok := totals[entryType]
	if !ok {
		t.Fatalf("dashboard omitted test entry type %q: %#v", entryType, totals)
	}
	if got.Cents() != 500 {
		t.Fatalf("business-local day total=%d cents, want 500", got.Cents())
	}
}
