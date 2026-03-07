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

func formatRFC3339InBeijing(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(beijingLocation).Format(time.RFC3339)
}
