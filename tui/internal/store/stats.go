package store

import (
	"math"
	"slices"
	"time"
)

// window is the span the stats strip covers.
const window = 60 * time.Second

// Strip is the stats strip: requests per second over the window, the median and 95th percentile
// of their milliseconds, and the share of them (in percent) that answered with a status of 500
// or more.
type Strip struct {
	ReqPerSec, P50, P95, ErrPct float64
}

// Stats is the strip over the requests of rs whose time is in (now-60s, now]. Percentiles are by
// nearest rank. With no request in the window every figure is zero.
func Stats(rs []Request, now time.Time) Strip {
	var ms []float64
	errs := 0
	for _, r := range rs {
		if !r.Time.After(now.Add(-window)) || r.Time.After(now) {
			continue
		}
		ms = append(ms, r.Ms)
		if r.Status >= 500 {
			errs++
		}
	}
	if len(ms) == 0 {
		return Strip{}
	}
	slices.Sort(ms)
	n := float64(len(ms))
	return Strip{ReqPerSec: n / window.Seconds(), P50: rank(ms, 0.50), P95: rank(ms, 0.95), ErrPct: 100 * float64(errs) / n}
}

// rank is the nearest-rank percentile p of the sorted values.
func rank(sorted []float64, p float64) float64 {
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[max(i, 0)]
}
