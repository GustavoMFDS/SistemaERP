package platform

import (
	"time"
	_ "time/tzdata" // Embed the IANA database for native Windows deployments.
)

// BusinessTimezone matches the current retail ranking and accounts policy.
const BusinessTimezone = "America/Sao_Paulo"

var businessLocation = func() *time.Location {
	location, err := time.LoadLocation(BusinessTimezone)
	if err != nil {
		panic("embedded business timezone unavailable")
	}
	return location
}()

// BusinessDate returns a calendar date, independent of the machine timezone.
func BusinessDate(now time.Time) string {
	return now.In(businessLocation).Format("2006-01-02")
}
