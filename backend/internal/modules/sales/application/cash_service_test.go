package application

import (
	"context"
	"testing"

	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
)

func TestCashMovementRequestHashIsDeterministic(t *testing.T) {
	notes := "troco adicional"
	req := CashMovementRequest{
		Type:   "supply",
		Amount: platform.NewMoneyCents(1250),
		Notes:  &notes,
	}

	first, err := cashMovementRequestHash("cash-session-1", req)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := cashMovementRequestHash("cash-session-1", req)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first != second {
		t.Fatalf("same normalized request must produce same hash: first=%s second=%s", first, second)
	}
}

func TestCashMovementRequestHashChangesWithRequestIdentity(t *testing.T) {
	notes := "troco adicional"
	base := CashMovementRequest{
		Type:   "supply",
		Amount: platform.NewMoneyCents(1250),
		Notes:  &notes,
	}
	baseHash, err := cashMovementRequestHash("cash-session-1", base)
	if err != nil {
		t.Fatalf("base hash: %v", err)
	}

	otherNotes := "outro motivo"
	cases := []struct {
		name      string
		sessionID string
		req       CashMovementRequest
	}{
		{name: "session", sessionID: "cash-session-2", req: base},
		{name: "type", sessionID: "cash-session-1", req: CashMovementRequest{Type: "withdrawal", Amount: base.Amount, Notes: base.Notes}},
		{name: "amount", sessionID: "cash-session-1", req: CashMovementRequest{Type: base.Type, Amount: platform.NewMoneyCents(1300), Notes: base.Notes}},
		{name: "notes", sessionID: "cash-session-1", req: CashMovementRequest{Type: base.Type, Amount: base.Amount, Notes: &otherNotes}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cashMovementRequestHash(tc.sessionID, tc.req)
			if err != nil {
				t.Fatalf("hash: %v", err)
			}
			if got == baseHash {
				t.Fatalf("%s change must alter request hash", tc.name)
			}
		})
	}
}

type currentCashExpectedRepo struct {
	fakeCashRepo
	session    sales.CashSession
	payments   map[string]platform.Money
	supply     platform.Money
	withdrawal platform.Money
}

func (r currentCashExpectedRepo) GetOpenSession(context.Context, string) (sales.CashSession, error) {
	return r.session, nil
}

func (r currentCashExpectedRepo) GetSession(context.Context, db.DBTX, string, string) (sales.CashSession, error) {
	return r.session, nil
}

func (r currentCashExpectedRepo) SumPaymentsByMethod(context.Context, db.DBTX, string, string) (map[string]platform.Money, error) {
	return r.payments, nil
}

func (r currentCashExpectedRepo) SumMovements(context.Context, db.DBTX, string, string) (platform.Money, platform.Money, error) {
	return r.supply, r.withdrawal, nil
}

func TestCurrentSessionWithExpectedCashIncludesCashMovementsOnly(t *testing.T) {
	repo := currentCashExpectedRepo{
		session: sales.CashSession{
			ID: "cash-1", Status: "open", OpeningAmount: platform.NewMoneyCents(2500),
		},
		payments: map[string]platform.Money{
			"cash": platform.NewMoneyCents(1000),
			"pix":  platform.NewMoneyCents(5000),
		},
		supply:     platform.NewMoneyCents(300),
		withdrawal: platform.NewMoneyCents(50),
	}
	service := NewCashService(fakeUOW{}, repo, nil, nil, nil, nil)

	_, got, err := service.CurrentSessionWithExpectedCash(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("current session expected cash: %v", err)
	}
	want := platform.NewMoneyCents(3750)
	if got != want {
		t.Fatalf("expected cash = %s, want %s", got, want)
	}
}
