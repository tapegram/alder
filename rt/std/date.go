package std

import (
	"fmt"
	"time"

	"alder/rt"
)

// DateFromText parses "YYYY-MM-DD"; any other shape or an impossible date
// is Nothing.
func DateFromText(s string) rt.Maybe[rt.Date] {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return rt.Nothing[rt.Date]()
	}
	return rt.Just(rt.DateOf(t))
}

// DateToText renders "YYYY-MM-DD".
func DateToText(d rt.Date) string { return fmt.Sprintf("%04d-%02d-%02d", d.Y, d.M, d.D) }

// DateFormat is `Date.format layout d` with a Go layout ("Jan 2, 2006").
func DateFormat(layout string, d rt.Date) string { return rt.Format(layout, d) }

func DateAddDays(n int64, d rt.Date) rt.Date { return rt.DateOf(d.Time().AddDate(0, 0, int(n))) }

// DateDiffDays is `Date.diffDays a b`: the days from a to b (b − a).
func DateDiffDays(a, b rt.Date) int64 { return int64(b.Time().Sub(a.Time()).Hours() / 24) }

func DateYear(d rt.Date) int64 { return int64(d.Y) }

func DateMonth(d rt.Date) int64 { return int64(d.M) }

func DateDay(d rt.Date) int64 { return int64(d.D) }

// TimeEpoch is `Time.epoch`: 1970-01-01T00:00:00Z.
func TimeEpoch() time.Time { return time.Unix(0, 0).UTC() }

// TimeToMillis is milliseconds since Time.epoch.
func TimeToMillis(t time.Time) int64 { return t.UnixMilli() }

// TimeFromMillis is the UTC instant ms milliseconds after Time.epoch.
func TimeFromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// TimeAddMillis is t moved by ms milliseconds.
func TimeAddMillis(ms int64, t time.Time) time.Time {
	return t.Add(time.Duration(ms) * time.Millisecond)
}

// TimeDiffMillis is `Time.diffMillis a b`: b − a in whole milliseconds, like Date.diffDays.
func TimeDiffMillis(a, b time.Time) int64 { return b.Sub(a).Milliseconds() }

// TimeToDate is the UTC calendar date of t.
func TimeToDate(t time.Time) rt.Date { return rt.DateOf(t) }

// TimeFormat is `Time.format layout t` with a Go layout, in UTC.
func TimeFormat(layout string, t time.Time) string { return t.UTC().Format(layout) }

// TimeAgo is `Time.ago now then`: "just now" under a minute, otherwise the
// largest whole unit (minute, hour, day, month = 30 days, year = 365 days),
// as "3 hours ago", or "in 3 hours" when then is after now.
func TimeAgo(now, then time.Time) string {
	d := now.Sub(then)
	future := d < 0
	if future {
		d = -d
	}
	units := []struct {
		name string
		size time.Duration
	}{
		{"year", 365 * 24 * time.Hour},
		{"month", 30 * 24 * time.Hour},
		{"day", 24 * time.Hour},
		{"hour", time.Hour},
		{"minute", time.Minute},
	}
	for _, u := range units {
		if n := int64(d / u.size); n >= 1 {
			phrase := fmt.Sprintf("%d %s", n, u.name)
			if n > 1 {
				phrase += "s"
			}
			if future {
				return "in " + phrase
			}
			return phrase + " ago"
		}
	}
	return "just now"
}

// DateFromParts builds a Date from year, month and day; an impossible date is
// Nothing (no normalisation: month 13 is not January).
func DateFromParts(p struct{ Year, Month, Day int64 }) rt.Maybe[rt.Date] {
	if p.Month < 1 || p.Month > 12 || p.Day < 1 || p.Day > 31 {
		return rt.Nothing[rt.Date]()
	}
	t := time.Date(int(p.Year), time.Month(p.Month), int(p.Day), 0, 0, 0, 0, time.UTC)
	if int64(t.Year()) != p.Year || int64(t.Month()) != p.Month || int64(t.Day()) != p.Day {
		return rt.Nothing[rt.Date]()
	}
	return rt.Just(rt.DateOf(t))
}

// DateToParts is the year, month and day of d.
func DateToParts(d rt.Date) struct{ Year, Month, Day int64 } {
	return struct{ Year, Month, Day int64 }{int64(d.Y), int64(d.M), int64(d.D)}
}

// DateEpoch is `Date.epoch`: 1970-01-01.
func DateEpoch() rt.Date { return rt.Date{Y: 1970, M: 1, D: 1} }

// DateWeekday is the ISO weekday of d: 1 = Monday … 7 = Sunday.
func DateWeekday(d rt.Date) int64 {
	w := int64(d.Time().Weekday())
	if w == 0 {
		return 7
	}
	return w
}

// DateAddMonths moves d by n months, clamping the day to the target month's
// length (Jan 31 + 1 month = Feb 28/29). It avoids AddDate, which normalises.
func DateAddMonths(n int64, d rt.Date) rt.Date {
	total := int64(d.Y)*12 + int64(d.M) - 1 + n
	y := dateFloorDiv(total, 12)
	m := total - y*12 + 1
	last := int64(time.Date(int(y), time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day())
	day := int64(d.D)
	if day > last {
		day = last
	}
	return rt.Date{Y: int(y), M: int(m), D: int(day)}
}

// DateAddYears is DateAddMonths (12 * n): Feb 29 becomes Feb 28 in a non-leap year.
func DateAddYears(n int64, d rt.Date) rt.Date { return DateAddMonths(12*n, d) }

// TimeFromDate is midnight UTC at the start of d.
func TimeFromDate(d rt.Date) time.Time { return d.Time() }

func dateFloorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
