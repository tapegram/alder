package std

import (
	"math"
	"strconv"
	"strings"
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

// IntClamp is `Int.clamp { min, max } n`, checked like Elm's clamp: below
// min gives min, else above max gives max, else n.
func IntClamp(r struct {
	Min int64
	Max int64
}, n int64) int64 {
	if n < r.Min {
		return r.Min
	}
	if n > r.Max {
		return r.Max
	}
	return n
}

// FloatRound rounds half away from zero.
func FloatRound(f float64) int64 { return int64(math.Round(f)) }

func FloatFloor(f float64) int64 { return int64(math.Floor(f)) }

func FloatCeil(f float64) int64 { return int64(math.Ceil(f)) }

func FloatMin(a, b float64) float64 { return math.Min(a, b) }

func FloatMax(a, b float64) float64 { return math.Max(a, b) }

func FloatAbs(f float64) float64 { return math.Abs(f) }

// FloatFormat is `Float.format { decimals } f`: fixed-point with exactly
// that many decimals (negative counts as 0). It rounds the shortest decimal
// form of f half away from zero, like Float.round (so 1.005 → "1.01"), and
// never prints a negative zero.
func FloatFormat(o struct{ Decimals int64 }, f float64) string {
	dec := int(max(o.Decimals, 0))
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return strconv.FormatFloat(f, 'f', dec, 64)
	}
	whole, frac, _ := strings.Cut(strconv.FormatFloat(math.Abs(f), 'f', -1, 64), ".")
	up := len(frac) > dec && frac[dec] >= '5'
	frac = (frac + strings.Repeat("0", dec))[:dec]
	digits := []byte(whole + frac)
	if up {
		digits = incDecimal(digits)
	}
	out := string(digits[:len(digits)-dec])
	if dec > 0 {
		out += "." + string(digits[len(digits)-dec:])
	}
	if f < 0 && strings.Trim(string(digits), "0") != "" {
		out = "-" + out
	}
	return out
}

// incDecimal adds one to a string of decimal digits.
func incDecimal(d []byte) []byte {
	for i := len(d) - 1; i >= 0; i-- {
		if d[i] < '9' {
			d[i]++
			return d
		}
		d[i] = '0'
	}
	return append([]byte{'1'}, d...)
}

// BoolToText renders the Alder constructor name: "True" or "False".
func BoolToText(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// PreludeNot is Prelude's `not`, for when it is passed as a value; applied
// uses may be emitted as `!`.
func PreludeNot(b bool) bool { return !b }
