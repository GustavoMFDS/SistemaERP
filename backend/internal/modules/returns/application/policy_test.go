package application

import (
	"testing"
	"time"
)

func stringPtr(v string) *string { return &v }
func intPtr(v int) *int { return &v }

func TestVoluntaryReturnPolicySnapshot(t *testing.T) {
	at := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct{name string;p *string;days *int;now time.Time;want bool}{
		{"historic",nil,nil,at.Add(900*24*time.Hour),true},
		{"always",stringPtr("allowed"),nil,at.Add(900*24*time.Hour),true},
		{"never",stringPtr("denied"),nil,at.Add(time.Hour),false},
		{"limit inclusive",stringPtr("within_days"),intPtr(7),at.Add(7*24*time.Hour),true},
		{"limit expired",stringPtr("within_days"),intPtr(7),at.Add(7*24*time.Hour+time.Second),false},
		{"invalid days",stringPtr("within_days"),nil,at.Add(time.Hour),false},
		{"invalid mode",stringPtr("invalid"),nil,at.Add(time.Hour),false},
	}
	for _, tc := range cases {
		t.Run(tc.name,func(t *testing.T){
			if got:=voluntaryReturnAllowed(tc.p,tc.days,at,tc.now);got!=tc.want{
				t.Fatalf("allowed=%t, want=%t",got,tc.want)
			}
		})
	}
}
