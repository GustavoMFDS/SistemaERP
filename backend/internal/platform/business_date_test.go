package platform

import (
	"testing"
	"time"
)

func TestBusinessDateUsesSaoPauloMidnight(t *testing.T) {
	for _, test := range []struct {
		instant string
		date    string
	}{
		{"2026-11-25T02:59:59Z", "2026-11-24"},
		{"2026-11-25T03:00:00Z", "2026-11-25"},
		{"2026-11-25T11:00:00+09:00", "2026-11-24"},
	} {
		instant, err := time.Parse(time.RFC3339, test.instant)
		if err != nil {
			t.Fatal(err)
		}
		if got := BusinessDate(instant); got != test.date {
			t.Errorf("BusinessDate(%s)=%s want %s", test.instant, got, test.date)
		}
	}
}
