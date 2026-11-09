package syntax

import (
	"strconv"
	"strings"
)

// EveryDuration reads the literal of an `every="…"` attribute (#160): a whole number above 0
// without leading zeros, then `ms`, `s` or `m` (`^[1-9][0-9]*(ms|s|m)$`). It returns the
// duration as Datastar's interval modifier takes it and whether lit matches. Datastar's parser
// has no minutes (it reads `1m` as 1 ms), so `Nm` is given as `<N*60>s`; `ms` and `s` pass
// through unchanged.
func EveryDuration(lit string) (string, bool) {
	num, unit := lit, ""
	for _, u := range []string{"ms", "s", "m"} {
		if strings.HasSuffix(lit, u) {
			num, unit = strings.TrimSuffix(lit, u), u
			break
		}
	}
	if unit == "" || num == "" || num[0] < '1' || num[0] > '9' {
		return "", false
	}
	for i := 1; i < len(num); i++ {
		if num[i] < '0' || num[i] > '9' {
			return "", false
		}
	}
	if unit != "m" {
		return lit, true
	}
	n, err := strconv.Atoi(num)
	if err != nil || n > (1<<62)/60 {
		return "", false
	}
	return strconv.Itoa(n*60) + "s", true
}
