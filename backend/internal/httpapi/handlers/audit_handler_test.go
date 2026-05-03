package handlers

import (
	"testing"

	"github.com/example/sistemaemgo/internal/modules/audit"
)

func TestValidateAuditFilter(t *testing.T) {
	valid := audit.ListFilter{
		ActorUserID: "11111111-1111-1111-1111-111111111111",
		Outcome:     "success",
		From:        "2026-01-01T00:00:00Z",
		To:          "2026-01-31T23:59:59Z",
	}
	if err := validateAuditFilter(valid); err != nil {
		t.Fatalf("expected valid filter: %v", err)
	}

	for _, tc := range []audit.ListFilter{
		{ActorUserID: "not-a-uuid"},
		{Outcome: "maybe"},
		{From: "2026-01-01"},
	} {
		if err := validateAuditFilter(tc); err == nil {
			t.Fatalf("expected invalid filter %#v to fail", tc)
		}
	}
}
