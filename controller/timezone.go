package main

import "time"

const beijingTZName = "Asia/Shanghai"

var beijingLocation = func() *time.Location {
	loc, err := time.LoadLocation(beijingTZName)
	if err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*60*60)
}()

func setDefaultTimezone() {
	time.Local = beijingLocation
}

func nowInBeijing() time.Time {
	return time.Now().In(beijingLocation)
}

func inBeijing(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.In(beijingLocation)
}

func formatRFC3339InBeijing(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return inBeijing(t).Format(time.RFC3339)
}

func formatDisplayTimeInBeijing(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return inBeijing(t).Format("2006-01-02 15:04:05")
}

func beijingOffsetMinutes(t time.Time) int {
	_, offset := inBeijing(t).Zone()
	return offset / 60
}
