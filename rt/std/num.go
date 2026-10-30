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

// FloatSqrt is the square root of f.
func FloatSqrt(f float64) float64 { return math.Sqrt(f) }

// FloatExp is e raised to the power f.
func FloatExp(f float64) float64 { return math.Exp(f) }

// FloatLog is the natural logarithm of f; log of a negative is NaN.
func FloatLog(f float64) float64 { return math.Log(f) }

// FloatPow is base raised to the power exp.
func FloatPow(base, exp float64) float64 { return math.Pow(base, exp) }

// FloatTruncate rounds toward zero.
func FloatTruncate(f float64) int64 { return int64(math.Trunc(f)) }

// FloatClamp is `Float.clamp { min, max } f`, checked like IntClamp: below
// min gives min, else above max gives max, else f (NaN stays NaN).
func FloatClamp(r struct {
	Min float64
	Max float64
}, f float64) float64 {
	if f < r.Min {
		return r.Min
	}
	if f > r.Max {
		return r.Max
	}
	return f
}

func FloatPi() float64 { return math.Pi }

func FloatE() float64 { return math.E }

func FloatIsNaN(f float64) bool { return math.IsNaN(f) }

func FloatIsInfinite(f float64) bool { return math.IsInf(f, 0) }

// FloatLogBase is `Float.logBase base x`, the logarithm of x in the given base.
func FloatLogBase(base, x float64) float64 { return math.Log(x) / math.Log(base) }

func FloatSin(f float64) float64 { return math.Sin(f) }

func FloatCos(f float64) float64 { return math.Cos(f) }

func FloatTan(f float64) float64 { return math.Tan(f) }

func FloatAsin(f float64) float64 { return math.Asin(f) }

func FloatAcos(f float64) float64 { return math.Acos(f) }

func FloatAtan(f float64) float64 { return math.Atan(f) }

// FloatAtan2 is `Float.atan2 y x`.
func FloatAtan2(y, x float64) float64 { return math.Atan2(y, x) }

// IntPow is `Int.pow base exp` by repeated squaring with wrapping
// multiplication; a negative exponent gives 0.
func IntPow(base, exp int64) int64 {
	if exp < 0 {
		return 0
	}
	result := int64(1)
	for exp > 0 {
		if exp&1 == 1 {
			result *= base
		}
		base *= base
		exp >>= 1
	}
	return result
}

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
