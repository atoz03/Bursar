package main

import (
	"testing"
	"time"
)

func TestEndOfDateForPostgresStaysInsideSelectedDay(t *testing.T) {
	start := time.Date(2026, 7, 31, 0, 0, 0, 0, beijingLocation)
	got := endOfDateForPostgres(start)
	want := time.Date(2026, 7, 31, 23, 59, 59, 999999000, beijingLocation)
	if !got.Equal(want) {
		t.Fatalf("end of day mismatch: got %s want %s", got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func TestParseUsageDateRangeUsesPostgresPrecision(t *testing.T) {
	setDefaultTimezone()
	from, hasFrom, to, hasTo, err := parseUsageRange("2026-07-01", "2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	if !hasFrom || !hasTo || from.Format("2006-01-02") != "2026-07-01" || to.Format("2006-01-02") != "2026-07-31" {
		t.Fatalf("unexpected range from=%s to=%s", from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano))
	}
	if to.Nanosecond() != 999999000 {
		t.Fatalf("unexpected PostgreSQL end precision: %d", to.Nanosecond())
	}
}
