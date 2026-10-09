package std

import (
	"math"
	"strconv"
)

func IntToFloat(n int64) float64 { return float64(n) }

func IntMin(a, b int64) int64 { return min(a, b) }

func IntMax(a, b int64) int64 { return max(a, b) }

func IntAbs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// IntClamp is `Int.clamp { min, max } n`.
func IntClamp(r struct {
	Min int64
	Max int64
}, n int64) int64 {
	return max(r.Min, min(r.Max, n))
}

// FloatRound rounds half away from zero.
func FloatRound(f float64) int64 { return int64(math.Round(f)) }

func FloatFloor(f float64) int64 { return int64(math.Floor(f)) }

func FloatCeil(f float64) int64 { return int64(math.Ceil(f)) }

func FloatMin(a, b float64) float64 { return math.Min(a, b) }

func FloatMax(a, b float64) float64 { return math.Max(a, b) }

func FloatAbs(f float64) float64 { return math.Abs(f) }

// FloatFormat is `Float.format { decimals } f`: fixed-point with exactly
// that many decimals (negative counts as 0), rounding as strconv does.
func FloatFormat(o struct{ Decimals int64 }, f float64) string {
	return strconv.FormatFloat(f, 'f', int(max(o.Decimals, 0)), 64)
}

// BoolToText renders the Alder constructor name: "True" or "False".
func BoolToText(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
