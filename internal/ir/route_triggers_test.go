package ir

import (
	"reflect"
	"strings"
	"testing"
)

// hattrs collects every HAttr reachable from v.
func hattrs(v any) []HAttr {
	var out []HAttr
	seen := map[uintptr]bool{}
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Pointer:
			if rv.IsNil() || seen[rv.Pointer()] {
				return
			}
			seen[rv.Pointer()] = true
			walk(rv.Elem())
		case reflect.Interface:
			if !rv.IsNil() {
				walk(rv.Elem())
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < rv.Len(); i++ {
				walk(rv.Index(i))
			}
		case reflect.Map:
			for _, k := range rv.MapKeys() {
				walk(rv.MapIndex(k))
			}
		case reflect.Struct:
			if a, ok := rv.Interface().(HAttr); ok {
				out = append(out, a)
			}
			for i := 0; i < rv.NumField(); i++ {
				if rv.Type().Field(i).IsExported() {
					walk(rv.Field(i))
				}
			}
		}
	}
	walk(reflect.ValueOf(v))
	return out
}

func triggerApp(every string) map[string]string {
	return map[string]string{
		"App.alder": redirectApp["App.alder"],
		"Stats.alder": `module Stats

route Stats = GET "/stats" { }
route Results = GET "/results" { q: Text = "" }

Stats.load : Stats.Params -> Session -> Query Int
Stats.load params session = Query.succeed 1

Stats.view : Int -> Session -> Html
Stats.view n session = <div id="box" load={Stats {}} every="` + every + `"><input name="q" input={Results { q: "x" }} /></div>

Results.load : Results.Params -> Session -> Query Text
Results.load params session = Query.succeed params.q

Results.view : Text -> Session -> Html
Results.view q session = <p>{q}</p>
`,
	}
}

func TestLower_RouteTriggers(t *testing.T) {
	p := lowerDir(t, writeApp(t, triggerApp("5s")))
	var load, input []HAttr
	for _, a := range hattrs(p) {
		switch {
		case a.Name == "every":
			t.Errorf("the every attribute is kept: %+v", a)
		case a.Kind == AttrLoad:
			load = append(load, a)
		case a.Kind == AttrInput:
			input = append(input, a)
		}
	}
	if len(load) != 1 || load[0].Every != "5s" {
		t.Errorf("load attrs: %+v", load)
	}
	if len(input) != 1 || input[0].Key != "q" {
		t.Errorf("input attrs: %+v", input)
	}
	if d := Dump(p); !strings.Contains(d, "(attr load load ") || !strings.Contains(d, "(attr input input") {
		t.Errorf("dump lacks the load or input attr:\n%s", d)
	}
}

func TestLower_EveryMinutes(t *testing.T) {
	for every, want := range map[string]string{"2m": "120s", "500ms": "500ms", "1s": "1s"} {
		p := lowerDir(t, writeApp(t, triggerApp(every)))
		got := ""
		for _, a := range hattrs(p) {
			if a.Kind == AttrLoad {
				got = a.Every
			}
		}
		if got != want {
			t.Errorf("every=%q lowers to %q, want %q", every, got, want)
		}
	}
}
