// Package dates parses the --since/--until cutoff values shared by the CLI
// and the desktop app (ruling R-D6): one implementation, identical semantics
// on both surfaces.
package dates

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseCutoff parses --since/--until values: exactly Nd, Nw (relative to now)
// or YYYY-MM-DD (local date); nothing else (controller ruling).
// endOfDay widens a plain date to the last nanosecond of that day so
// --until 2026-01-01 includes that whole day.
func ParseCutoff(flag, value string, endOfDay bool) (time.Time, error) {
	invalid := fmt.Errorf("invalid --%s value %q: use Nd, Nw or YYYY-MM-DD (e.g. 7d, 2w, 2026-01-01)", flag, value)

	if t, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
		if endOfDay {
			return t.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
		}
		return t, nil
	}

	for _, suffix := range []struct {
		unit string
		days int
	}{
		{"d", 1},
		{"w", 7},
	} {
		if n, ok := cutPositiveInt(value, suffix.unit); ok {
			return time.Now().AddDate(0, 0, -n*suffix.days), nil
		}
		if strings.HasSuffix(value, suffix.unit) {
			return time.Time{}, invalid // right unit, bad number
		}
	}
	return time.Time{}, invalid
}

// cutPositiveInt parses "<digits><unit>" into a positive day count.
func cutPositiveInt(value, unit string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSuffix(value, unit))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
