package timezone

import (
	"fmt"
	"time"

	"github.com/tkuchiki/go-timezone"
)

// NameForTime returns an IANA name that matches the timezone information on the given time, for example
// Asia/Shanghai. If there are multiple IANA names that match the timezone on the given time, this function
// makes no guarantees about which one is returned.
func IANANameForTime(t time.Time) (string, error) {
	isDST := isTimeDST(t)
	abbrv, offset := t.Zone()
	tz := timezone.New()
	timezones, err := tz.GetTimezones(abbrv)
	if err != nil {
		return "", fmt.Errorf("unable to find timezone for %v (%d): %w", abbrv, offset, err)
	}
	for _, timezone := range timezones {
		info, err := tz.GetTzInfo(timezone)
		if err != nil {
			continue
		}
		if !info.IsDeprecated() {
			if isDST && info.DaylightOffset() == offset {
				return timezone, nil
			}
			if !isDST && info.StandardOffset() == offset {
				return timezone, nil
			}
		}
	}

	return "", fmt.Errorf("unable to find timezone for %v (%d)", abbrv, offset)
}

// isTimeDST returns true if time t occurs within daylight saving time
// for its time zone.
// from https://stackoverflow.com/questions/53046636/how-to-check-whether-current-local-time-is-dst
func isTimeDST(t time.Time) bool {
	// If the most recent (within the last year) clock change
	// was forward then assume the change was from std to dst.
	hh, mm, _ := t.UTC().Clock()
	tClock := hh*60 + mm
	for m := -1; m > -12; m-- {
		// assume dst lasts for least one month
		hh, mm, _ := t.AddDate(0, m, 0).UTC().Clock()
		clock := hh*60 + mm
		if clock != tClock {
			if clock > tClock {
				// std to dst
				return true
			}
			// dst to std
			return false
		}
	}
	// assume no dst
	return false
}
