package std

import (
	"math"
	"testing"

	"alder/rt"
)

func TestNumBatch(t *testing.T) {
	type mm = struct{ Min, Max float64 }
	eq(t, "truncate pos", FloatTruncate(2.7), int64(2))
	eq(t, "truncate neg", FloatTruncate(-2.7), int64(-2))
	eq(t, "clamp below", FloatClamp(mm{0, 1}, -0.5), 0.0)
	eq(t, "clamp above", FloatClamp(mm{0, 1}, 1.5), 1.0)
	eq(t, "clamp inside", FloatClamp(mm{0, 1}, 0.25), 0.25)
	eq(t, "clamp min > max like Int.clamp", FloatClamp(mm{5, 1}, 3), 5.0)
	if !math.IsNaN(FloatClamp(mm{0, 1}, math.NaN())) {
		t.Error("clamp NaN should be NaN")
	}
	eq(t, "pi", FloatPi(), math.Pi)
	eq(t, "e", FloatE(), math.E)
	eq(t, "isNaN", FloatIsNaN(math.NaN()), true)
	eq(t, "isNaN finite", FloatIsNaN(1), false)
	eq(t, "isInfinite", FloatIsInfinite(math.Inf(-1)), true)
	eq(t, "isInfinite finite", FloatIsInfinite(1), false)
	if math.Abs(FloatLogBase(10, 1000)-3) > 1e-12 {
		t.Error("logBase 10 1000")
	}
	eq(t, "atan2", FloatAtan2(1, 1), math.Pi/4)
	eq(t, "sin", FloatSin(0), 0.0)
	eq(t, "cos", FloatCos(0), 1.0)
	eq(t, "tan", FloatTan(0), 0.0)
	eq(t, "asin", FloatAsin(0), 0.0)
	eq(t, "acos", FloatAcos(1), 0.0)
	eq(t, "atan", FloatAtan(0), 0.0)
	eq(t, "pow", IntPow(2, 10), int64(1024))
	eq(t, "pow zero exp", IntPow(0, 0), int64(1))
	eq(t, "pow negative exp", IntPow(2, -1), int64(0))
	eq(t, "pow negative base", IntPow(-3, 3), int64(-27))
	eq(t, "pow wraps like *", IntPow(2, 64), int64(0))
}

func TestTextToFloatExponent(t *testing.T) {
	eq(t, "1.5e-3", TextToFloat("1.5e-3"), rt.Just(0.0015))
	eq(t, "2E+3", TextToFloat("2E+3"), rt.Just(2000.0))
}
