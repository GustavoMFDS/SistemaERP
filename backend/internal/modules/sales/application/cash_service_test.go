package application

import (
	"testing"

	"github.com/example/sistemaemgo/internal/platform"
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
