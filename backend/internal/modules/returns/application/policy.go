package application

import "time"

// A historic sale without a policy snapshot keeps its existing return path.
// Until each store has a configured civil-day time zone, the deadline uses
// elapsed 24-hour periods from the recorded sale time.
func voluntaryReturnAllowed(policy *string, days *int, finalizedAt, now time.Time) bool {
	if policy == nil {
		return true
	}
	switch *policy {
	case "allowed":
		return true
	case "denied":
		return false
	case "within_days":
		if days == nil || *days < 1 || *days > 3650 || finalizedAt.IsZero() {
			return false
		}
		return !now.After(finalizedAt.Add(time.Duration(*days) * 24 * time.Hour))
	default:
		return false
	}
}
