package team

import (
	"testing"
)

func TestInviteInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name, email, role string
		ok                bool
	}{
		{name: "Joana Silva", email: "joana@example.com", role: "cashier", ok: true},
		{name: "Joana Silva", email: "joana@example.com", role: "manager", ok: true},
		{name: "Joana", email: "joana@example.com", role: "admin"},
		{name: "J", email: "joana@example.com", role: "manager"},
		{name: "Joana Silva", email: "bad email", role: "cashier"},
		{name: "Joana\nSilva", email: "joana@example.com", role: "cashier"},
		{name: "Joana Silva", email: "other@example.com\nBcc:admin@example.com", role: "cashier"},
	} {
		if validNewInvite(tc.name, tc.email, tc.role) != tc.ok {
			t.Errorf("unexpected invite validation for role %q and email %q", tc.role, tc.email)
		}
	}
}

func TestOnlyOperationalRolesCanBeAssigned(t *testing.T) {
	for _, role := range []string{"cashier", "manager"} {
		if !permittedRole(role) {
			t.Errorf("role must be assignable: %s", role)
		}
	}
	for _, role := range []string{"admin", "owner", "product:write", "audit:read", "", "manager;admin"} {
		if permittedRole(role) {
			t.Errorf("privileged or invalid role must be rejected: %q", role)
		}
	}
}
