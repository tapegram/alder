package syntax

import "testing"

func TestEveryDuration(t *testing.T) {
	ok := map[string]string{"5s": "5s", "500ms": "500ms", "2m": "120s", "1m": "60s", "10s": "10s"}
	for lit, want := range ok {
		if got, valid := EveryDuration(lit); !valid || got != want {
			t.Errorf("EveryDuration(%q) = %q, %v; want %q, true", lit, got, valid, want)
		}
	}
	for _, lit := range []string{"0s", "5", "05s", "", "s", "ms", "1.5s", "-1s", "5 s", "99999999999999999999m"} {
		if got, valid := EveryDuration(lit); valid {
			t.Errorf("EveryDuration(%q) = %q, true; want false", lit, got)
		}
	}
}

func TestEveryDuration_HoursDays(t *testing.T) {
	for lit, want := range map[string]string{"1h": "3600s", "2d": "172800s", "24h": "86400s"} {
		if got, valid := EveryDuration(lit); !valid || got != want {
			t.Errorf("EveryDuration(%q) = %q, %v; want %q, true", lit, got, valid, want)
		}
	}
	for _, lit := range []string{"0h", "01d", "h", "d", "1.5h", "99999999999999999999d"} {
		if got, valid := EveryDuration(lit); valid {
			t.Errorf("EveryDuration(%q) = %q, true; want false", lit, got)
		}
	}
}
