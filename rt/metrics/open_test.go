package metrics

import (
	"strings"
	"testing"
)

// "open" is refused where the one port is public: Render, Fly, a box deployed by alder. A token
// is accepted there.
func TestFromEnv_OpenRefusedOnPublicTargets(t *testing.T) {
	for marker, where := range map[string]string{"RENDER=true": "Render", "FLY_APP_NAME=blog": "Fly", "ALDER_DEPLOY_TARGET=ssh": "a box deployed by alder"} {
		k, v, _ := strings.Cut(marker, "=")
		c, err := fromEnv(env(map[string]string{"ALDER_METRICS": "open", k: v}))
		want := "ALDER_METRICS=open on " + where + ": the endpoint would be public (route patterns, step and table names, the build id, traffic): set a token of at least 16 characters instead"
		if c != nil || err == nil || err.Error() != want {
			t.Errorf("%s: %v, %v", marker, c, err)
		}
		if c, err := fromEnv(env(map[string]string{"ALDER_METRICS": tok, k: v})); c == nil || err != nil || c.Open() {
			t.Errorf("%s token: %v, %v", marker, c, err)
		}
	}
	if c, err := fromEnv(env(map[string]string{"ALDER_METRICS": "open"})); c == nil || err != nil || !c.Open() {
		t.Errorf("open off a target: %v, %v", c, err)
	}
}
