package check

import (
	"strings"
	"testing"
)

func TestHookDoc(t *testing.T) {
	want := map[string]string{
		"load":   "Params -> Session -> Query Model",
		"view":   "Model -> Session -> Html Msg",
		"update": "Msg -> Model -> Session -> Task Nav",
	}
	for name, sig := range want {
		got, role, ok := HookDoc(name)
		if !ok || got != sig {
			t.Errorf("HookDoc(%s) = %q, %v; want %q", name, got, ok, sig)
		}
		if !strings.HasPrefix(role, "Page hook. ") || !strings.Contains(role, "`"+sig+"`") {
			t.Errorf("HookDoc(%s) role %q: must start with \"Page hook. \" and quote the signature", name, role)
		}
	}
	if _, _, ok := HookDoc("Model"); ok {
		t.Error("Model is not a hook")
	}
	if _, _, ok := HookDoc(""); ok {
		t.Error("the empty name is not a hook")
	}
}
