package syntax

import "testing"

func TestEveryDuration(t *testing.T) {
	ok := map[string]string{"5s": "5s", "500ms": "500ms", "2m": "120s", "1m": "60s", "10s": "10s"}
	for lit, want := range ok {
		if got, valid := EveryDuration(lit); !valid || got != want {
			t.Errorf("EveryDuration(%q) = %q, %v; want %q, true", lit, got, valid, want)
		}
	}
	for _, lit := range []string{"0s", "5", "05s", "1h", "", "s", "ms", "1.5s", "-1s", "5 s", "99999999999999999999m"} {
		if got, valid := EveryDuration(lit); valid {
			t.Errorf("EveryDuration(%q) = %q, true; want false", lit, got)
		}
	}
}
