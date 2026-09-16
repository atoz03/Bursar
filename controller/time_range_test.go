package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func TestParseStatsRangeRejectsUnboundedSpan(t *testing.T) {
	setDefaultTimezone()
	gin.SetMode(gin.TestMode)
	parse := func(query string) error {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/admin/stats/daily?"+query, nil)
		_, _, err := parseStatsRange(c, 30)
		return err
	}
	tooOld := nowInBeijing().AddDate(-10, 0, 0).Format("2006-01-02")
	if err := parse("from=" + tooOld); err == nil || !strings.Contains(err.Error(), "统计区间不能超过") {
		t.Fatalf("a ten-year range must be rejected by the span limit, got %v", err)
	}
	from := nowInBeijing().AddDate(-1, 0, 0).Format("2006-01-02")
	if err := parse("from=" + from); err != nil {
		t.Fatalf("a one-year range must be accepted: %v", err)
	}
	if err := parse(""); err != nil {
		t.Fatalf("the default range must be accepted: %v", err)
	}
}
