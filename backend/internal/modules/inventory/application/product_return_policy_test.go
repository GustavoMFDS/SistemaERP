package application

import "testing"

func returnPolicyPtr(v string) *string { return &v }
func returnDaysPtr(v int) *int { return &v }

func TestValidateVoluntaryReturnPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		policy *string
		days *int
		valid bool
	}{
		{"legacy client", nil, nil, true},
		{"allowed", returnPolicyPtr("allowed"), nil, true},
		{"denied", returnPolicyPtr("denied"), nil, true},
		{"within days", returnPolicyPtr("within_days"), returnDaysPtr(7), true},
		{"no days", returnPolicyPtr("within_days"), nil, false},
		{"zero days", returnPolicyPtr("within_days"), returnDaysPtr(0), false},
		{"too many days", returnPolicyPtr("within_days"), returnDaysPtr(3651), false},
		{"extraneous days", returnPolicyPtr("denied"), returnDaysPtr(3), false},
		{"unknown mode", returnPolicyPtr("unknown"), nil, false},
		{"missing policy and days", nil, returnDaysPtr(7), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := ProductCreateRequest{ReturnPolicy: tc.policy, ReturnDays: tc.days}
			got := validateVoluntaryReturnPolicy(req) == nil
			if got != tc.valid {
				t.Fatalf("policy valid=%t, want %t", got, tc.valid)
			}
		})
	}
}
