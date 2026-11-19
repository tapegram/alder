package cron

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for _, ok := range []string{"0 3 * * *", "*/15 * * * *", "0 9-17 * * 1-5", "0 0 1,15 * *", "5/20 * * * *", "0 0 31 * *", "0 0 29 2 *", "0 0 31 2 1"} {
		if _, err := Parse(ok); err != nil {
			t.Errorf("Parse(%q): %v", ok, err)
		}
	}
	bad := []struct{ expr, field string }{
		{"", "minute"},
		{"0 3 * *", "day of week"},
		{"0 3 * * * *", "minute"},
		{"60 * * * *", "minute"},
		{"0 24 * * *", "hour"},
		{"0 25 * * *", "hour"},
		{"0 0 0 * *", "day of month"},
		{"0 0 32 * *", "day of month"},
		{"0 0 * 13 *", "month"},
		{"0 0 * JAN *", "month"},
		{"0 0 * * MON", "day of week"},
		{"0 0 * * 7", "day of week"},
		{"L * * * *", "minute"},
		{"0 0 L * *", "day of month"},
		{"5/0 * * * *", "minute"},
		{"*/-1 * * * *", "minute"},
		{"3-1 * * * *", "minute"},
		{"1,,2 * * * *", "minute"},
		{"1- * * * *", "minute"},
		{"0 0 31 2 *", "day of month"},
		{"0 0 30 2 *", "day of month"},
		{"0 0 31 4,6 *", "day of month"},
	}
	for _, tc := range bad {
		_, err := Parse(tc.expr)
		if err == nil || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("Parse(%q) = %v, want an error naming %q", tc.expr, err, tc.field)
		}
	}
}

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestNext(t *testing.T) {
	cases := []struct{ expr, after, want string }{
		{"0 3 * * *", "2026-03-01 02:59", "2026-03-01 03:00"},
		{"0 3 * * *", "2026-03-01 03:00", "2026-03-02 03:00"},
		{"*/15 * * * *", "2026-03-01 10:14", "2026-03-01 10:15"},
		{"*/15 * * * *", "2026-03-01 10:45", "2026-03-01 11:00"},
		{"0 0 29 2 *", "2026-03-01 00:00", "2028-02-29 00:00"},
		{"0 0 1 1 *", "2026-12-31 23:59", "2027-01-01 00:00"},
		{"30 9 * * 1-5", "2026-11-20 09:30", "2026-11-23 09:30"}, // Friday -> Monday
		// day of month and day of week both restricted: either matches (the cron OR rule)
		{"0 0 13 * 5", "2026-11-01 00:00", "2026-11-06 00:00"},   // Friday the 6th, before the 13th
		{"0 0 13 * 5", "2026-11-07 00:00", "2026-11-13 00:00"},   // the 13th, which is also a Friday
		{"0 0 14 * 5", "2026-11-07 00:00", "2026-11-13 00:00"},   // Friday the 13th, before the 14th
		{"0 0 14 * 5", "2026-11-13 00:00", "2026-11-14 00:00"},   // the 14th, a Saturday
		{"0 0 31 2 1", "2026-03-01 00:00", "2027-02-01 00:00"},   // never on the 31st; Mondays of February
		{"0 0 31 * */2", "2026-11-01 00:00", "2026-12-31 00:00"}, // `*/2` is not a restriction: both must match; Dec 31 2026 is a Thursday
	}
	for _, tc := range cases {
		s, err := Parse(tc.expr)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.expr, err)
		}
		if got := s.Next(at(tc.after)); !got.Equal(at(tc.want)) {
			t.Errorf("%q after %s = %s, want %s", tc.expr, tc.after, got.Format("2006-01-02 15:04"), tc.want)
		}
	}
}

func TestNext_StrictlyAfterAndUTC(t *testing.T) {
	s, _ := Parse("0 3 * * *")
	// 04:30 at +05:00 is 23:30 UTC the day before
	zone := time.FixedZone("x", 5*3600)
	got := s.Next(time.Date(2026, 3, 2, 4, 30, 0, 0, zone))
	if want := at("2026-03-02 03:00"); !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("got %v, want %v in UTC", got, want)
	}
	// seconds past the matching minute do not match it again
	got = s.Next(time.Date(2026, 3, 1, 3, 0, 30, 0, time.UTC))
	if want := at("2026-03-02 03:00"); !got.Equal(want) {
		t.Errorf("after 03:00:30 = %v, want %v", got, want)
	}
}

func TestNext_Bounded(t *testing.T) {
	// the longest wait that Parse allows: the 29th of February on a Monday in `*/…` form
	s, err := Parse("0 0 29 2 */3")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Next(at("2026-01-01 00:00")); got.IsZero() {
		t.Error("no match found")
	}
}
