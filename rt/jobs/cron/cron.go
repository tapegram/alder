// Package cron reads the five-field cron expressions of a job's `cron "…"` option (#168) and
// finds when one next matches. The fields are minute (0-59), hour (0-23), day of month
// (1-31), month (1-12) and day of week (0-6, Sunday is 0); each is a list of items
// separated by commas, and an item is a number, `*`, a range `a-b`, or either of those with a
// step: `*/15`, `10-40/5`, and `5/20` (from 5 to the field's end). There are no names
// (`MON`), no seconds, and no `L`, `W` or `#`. Every time is UTC.
//
// When both day of month and day of week are restricted (neither begins with `*`), a day
// matches if either does, as in standard cron: `0 0 13 * 5` runs on every 13th and on every
// Friday. If one of them begins with `*` (`*`, `*/2`), a day must match both.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed cron expression: a set of minutes, hours, days of month, months and
// days of week, each one bit per value.
type Schedule struct {
	minute, hour, dom, month, dow uint64
	domStar, dowStar              bool // the field begins with `*`: it does not restrict the day by the OR rule
}

// the fields in order, with their names (as an error names them) and ranges
var fields = [5]struct {
	name     string
	min, max int
}{{"minute", 0, 59}, {"hour", 0, 23}, {"day of month", 1, 31}, {"month", 1, 12}, {"day of week", 0, 6}}

// Parse reads expr. An error about a field names it: "minute", "hour", "day of month", "month"
// or "day of week"; an expression without five fields is an error that says how many it has
// and names none. A restricted day of month with a `*` day of week, none of whose days exists in
// the months listed (`0 0 31 2 *`), is an error naming "day of month"; with both day fields
// restricted (`0 0 31 2 1`) the day of week can still match, and it is accepted.
func Parse(expr string) (Schedule, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("a cron expression has five fields (minute, hour, day of month, month, day of week), not %d", len(parts))
	}
	var sets [5]uint64
	for i, p := range parts {
		set, err := parseField(p, fields[i].min, fields[i].max)
		if err != nil {
			return Schedule{}, fmt.Errorf("%s `%s`: %w", fields[i].name, p, err)
		}
		sets[i] = set
	}
	s := Schedule{minute: sets[0], hour: sets[1], dom: sets[2], month: sets[3], dow: sets[4],
		domStar: strings.HasPrefix(parts[2], "*"), dowStar: strings.HasPrefix(parts[4], "*")}
	if !s.domStar && s.dowStar && !s.domPossible() {
		return Schedule{}, fmt.Errorf("day of month `%s` never happens in the months `%s` (they have too few days); pick another day or month", parts[2], parts[3])
	}
	return s, nil
}

// domPossible reports whether some month of s has a day of month of s (February counts 29).
func (s Schedule) domPossible() bool {
	for m := 1; m <= 12; m++ {
		if s.month&(1<<m) != 0 && s.dom&(1<<(daysIn[m]+1)-2) != 0 {
			return true
		}
	}
	return false
}

var daysIn = [13]int{0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// parseField reads one field into a bitset of its values.
func parseField(p string, lo, hi int) (uint64, error) {
	var set uint64
	for _, item := range strings.Split(p, ",") {
		base, stepText, hasStep := strings.Cut(item, "/")
		step := 1
		if hasStep {
			n, ok := number(stepText)
			if !ok || n < 1 {
				return 0, fmt.Errorf("the step `%s` must be a whole number from 1", stepText)
			}
			step = n
		}
		from, to := lo, hi
		if base != "*" {
			a, b, isRange := strings.Cut(base, "-")
			n, ok := number(a)
			if !ok {
				return 0, fmt.Errorf("`%s` is not a number; use numbers, `*`, `,`, `-` and `/` (no names, `L`, `W` or `#`)", a)
			}
			from, to = n, n
			if isRange {
				if to, ok = number(b); !ok {
					return 0, fmt.Errorf("`%s` is not a number; use numbers, `*`, `,`, `-` and `/` (no names, `L`, `W` or `#`)", b)
				}
			} else if hasStep {
				to = hi // `5/20`: from 5 to the end
			}
			if from < lo || from > hi || to < lo || to > hi {
				return 0, fmt.Errorf("the values are %d to %d", lo, hi)
			}
			if from > to {
				return 0, fmt.Errorf("the range `%s` runs backwards", base)
			}
		}
		for v := from; v <= to; v += step {
			set |= 1 << v
		}
	}
	return set, nil
}

// number reads a whole number of digits only (no sign, no space).
func number(s string) (int, bool) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && len(s) <= 4
}

// matchDay reports whether the day of t is in s, by the rule of the package comment.
func (s Schedule) matchDay(t time.Time) bool {
	d := s.dom&(1<<t.Day()) != 0
	w := s.dow&(1<<int(t.Weekday())) != 0
	if s.domStar || s.dowStar {
		return d && w
	}
	return d || w
}

// searchYears bounds Next: the Gregorian calendar repeats every 400 years (146097 days, a whole
// number of weeks), so a day that matches once matches again within 400 years of any day, and
// the search through the 400th anniversary of its first day, inclusive, finds the next match of
// every schedule that has one. The longest wait in practice is 40 years (a 29 February on a
// Sunday, from 2088 to 2128).
const searchYears = 400

// Next is the first minute matching s strictly after `after`, in UTC, or the zero Time if none
// within searchYears, which happens only for a schedule that never matches (Parse refuses those,
// so for a parsed one this is a guard).
func (s Schedule) Next(after time.Time) time.Time {
	t := after.UTC().Truncate(time.Minute).Add(time.Minute)
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	end := day.AddDate(searchYears, 0, 0)
	for ; !day.After(end); day = day.AddDate(0, 0, 1) {
		if s.month&(1<<int(day.Month())) == 0 || !s.matchDay(day) {
			continue
		}
		for h := 0; h < 24; h++ {
			if s.hour&(1<<h) == 0 {
				continue
			}
			for m := 0; m < 60; m++ {
				if c := day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute); s.minute&(1<<m) != 0 && !c.Before(t) {
					return c
				}
			}
		}
	}
	return time.Time{}
}
