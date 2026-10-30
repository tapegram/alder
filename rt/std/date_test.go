package std

import (
	"testing"
	"time"

	"alder/rt"
)

func dt(t *testing.T, s string) rt.Date {
	t.Helper()
	m := DateFromText(s)
	if !m.Ok {
		t.Fatalf("bad date %q", s)
	}
	return m.Val
}

type dparts = struct{ Year, Month, Day int64 }

func TestDate_FromParts(t *testing.T) {
	eq(t, "ok", DateFromParts(dparts{2026, 10, 29}), rt.Just(dt(t, "2026-10-29")))
	eq(t, "leap", DateFromParts(dparts{2024, 2, 29}), rt.Just(dt(t, "2024-02-29")))
	for _, p := range []dparts{{2026, 13, 1}, {2026, 0, 1}, {2026, 2, 29}, {2026, 4, 31}, {2026, 1, 0}} {
		eq(t, "bad", DateFromParts(p), rt.Nothing[rt.Date]())
	}
}

func TestDate_ToParts(t *testing.T) {
	eq(t, "toParts", DateToParts(dt(t, "2026-10-29")), dparts{2026, 10, 29})
	d := dt(t, "2024-02-29")
	eq(t, "roundtrip", DateFromParts(DateToParts(d)), rt.Just(d))
}

func TestDate_Epoch(t *testing.T) {
	eq(t, "epoch", DateEpoch(), dt(t, "1970-01-01"))
}

func TestDate_Weekday(t *testing.T) {
	eq(t, "mon", DateWeekday(dt(t, "2026-10-26")), int64(1))
	eq(t, "sun", DateWeekday(dt(t, "2026-11-01")), int64(7))
	eq(t, "epoch", DateWeekday(dt(t, "1970-01-01")), int64(4))
}

func TestDate_AddMonths(t *testing.T) {
	cases := []struct {
		n        int64
		from, to string
	}{
		{1, "2026-01-31", "2026-02-28"},
		{1, "2024-01-31", "2024-02-29"},
		{-1, "2026-03-31", "2026-02-28"},
		{12, "2026-05-15", "2027-05-15"},
		{-13, "2026-01-15", "2024-12-15"},
		{0, "2026-01-31", "2026-01-31"},
	}
	for _, c := range cases {
		eq(t, c.from, DateAddMonths(c.n, dt(t, c.from)), dt(t, c.to))
	}
}

func TestDate_AddYears(t *testing.T) {
	eq(t, "1", DateAddYears(1, dt(t, "2024-02-29")), dt(t, "2025-02-28"))
	eq(t, "4", DateAddYears(4, dt(t, "2024-02-29")), dt(t, "2028-02-29"))
	eq(t, "-1", DateAddYears(-1, dt(t, "2026-06-01")), dt(t, "2025-06-01"))
}

func TestDate_TimeFromDate(t *testing.T) {
	got := TimeFromDate(dt(t, "1970-01-02"))
	eq(t, "ms", got, TimeFromMillis(86400000))
	if got.Location() != time.UTC {
		t.Errorf("location %v", got.Location())
	}
}
