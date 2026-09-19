package dates

import (
	"testing"
	"time"
)

// Unit tests moved with the parseCutoff extraction (R-D6); the CLI-level
// behavior net stays in internal/cli (commands_test.go & co).
func TestParseCutoffDateForm(t *testing.T) {
	got, err := ParseCutoff("since", "2026-01-01", false)
	if err != nil {
		t.Fatalf("date form rejected: %v", err)
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("since 2026-01-01 = %v, want %v", got, want)
	}
}

func TestParseCutoffUntilWidensToEndOfDay(t *testing.T) {
	got, err := ParseCutoff("until", "2026-01-01", true)
	if err != nil {
		t.Fatalf("date form rejected: %v", err)
	}
	want := time.Date(2026, 1, 1, 23, 59, 59, 999999999, time.Local)
	if !got.Equal(want) {
		t.Errorf("until 2026-01-01 = %v, want the last nanosecond of that day %v", got, want)
	}
}

func TestParseCutoffRelativeDays(t *testing.T) {
	before := time.Now()
	got, err := ParseCutoff("since", "7d", false)
	after := time.Now()
	if err != nil {
		t.Fatalf("7d rejected: %v", err)
	}
	if got.Before(before.AddDate(0, 0, -7)) || got.After(after.AddDate(0, 0, -7)) {
		t.Errorf("7d = %v, want now-7d (within the test window %v..%v)", got, before, after)
	}
}

func TestParseCutoffRelativeWeeks(t *testing.T) {
	got, err := ParseCutoff("since", "2w", false)
	if err != nil {
		t.Fatalf("2w rejected: %v", err)
	}
	want := time.Now().AddDate(0, 0, -14)
	if got.Sub(want).Abs() > time.Minute {
		t.Errorf("2w = %v, want ~now-14d (%v)", got, want)
	}
}

func TestParseCutoffInvalid(t *testing.T) {
	for _, value := range []string{"3x", "d", "-7d", "0d", "2026-13-01", "01.01.2026", ""} {
		if _, err := ParseCutoff("since", value, false); err == nil {
			t.Errorf("ParseCutoff(since, %q) accepted, want invalid", value)
		} else if want := `invalid --since value "` + value + `"`; len(err.Error()) < len(want) || err.Error()[:len(want)] != want {
			t.Errorf("ParseCutoff(since, %q) error = %q, want prefix %q", value, err.Error(), want)
		}
	}
}

func TestParseCutoffFlagNameInError(t *testing.T) {
	_, err := ParseCutoff("until", "nope", true)
	if err == nil {
		t.Fatal("invalid value accepted")
	}
	want := `invalid --until value "nope"`
	if err.Error()[:len(want)] != want {
		t.Errorf("error = %q, want prefix %q", err.Error(), want)
	}
}
