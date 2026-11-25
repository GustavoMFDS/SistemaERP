package bootstrapadmin

import "testing"

func TestClassifyStatus(t *testing.T) {
	fresh := statusCounts{companies: 1, placeholders: 1}
	committed := statusCounts{companies: 1, users: 1, memberships: 1, roles: 1, tenantRoles: 1, auditEvents: 1, matching: 1}
	tests := []struct {
		name   string
		counts statusCounts
		want   Status
	}{
		{"absent", fresh, AdminAbsent},
		{"committed", committed, AdminCommitted},
		{"empty database", statusCounts{}, AdminIndeterminate},
		{"orphan user", statusCounts{companies: 1, placeholders: 1, users: 1}, AdminIndeterminate},
		{"company only", statusCounts{companies: 1}, AdminIndeterminate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyStatus(tt.counts); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
	// No single missing/extra count in a committed snapshot may be accepted.
	for i := 0; i < 8; i++ {
		c := committed
		fields := []*int{&c.companies, &c.placeholders, &c.users, &c.memberships, &c.roles, &c.tenantRoles, &c.auditEvents, &c.matching}
		*fields[i] += 1
		if classifyStatus(c) != AdminIndeterminate {
			t.Fatalf("extra count %d accepted", i)
		}
		if *fields[i] > 1 {
			*fields[i] -= 2
			if classifyStatus(c) != AdminIndeterminate {
				t.Fatalf("missing count %d accepted", i)
			}
		}
	}
}
