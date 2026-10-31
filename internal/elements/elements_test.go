package elements

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// The embedded manifest declares exactly the first tier of `alder-ui` as the M7a spec lists it:
// each element's attributes (in order, the boolean ones marked) and slots.
func TestBuiltin_FirstTier(t *testing.T) {
	want := map[string]struct {
		attrs string // "name" or "name!" for a boolean
		slots string // "-" for the default slot
	}{
		"al-button":     {"variant size disabled! type full!", "-"},
		"al-input":      {"label placeholder disabled! invalid! help", ""},
		"al-textarea":   {"label placeholder disabled! invalid! help", ""},
		"al-select":     {"label disabled! invalid! help", ""},
		"al-checkbox":   {"label disabled! invalid! help", ""},
		"al-table":      {"striped! dense!", "-"},
		"al-card":       {"", "header - footer actions description action"},
		"al-badge":      {"variant", "-"},
		"al-alert":      {"variant heading dismissible!", "-"},
		"al-pagination": {"page pages", "prev - next"},
		"al-stack":      {"gap direction", "-"},
		"al-page":       {"width plain!", "-"},
		// The interactive tier (v0.2.0).
		"al-dialog":   {"heading size dismissible!", "header - actions description"},
		"al-popover":  {"placement offset anchor", "-"},
		"al-menu":     {"anchor", "-"},
		"al-tabs":     {"variant", "-"},
		"al-tab":      {"name selected!", "-"},
		"al-panel":    {"", "-"},
		"al-toast":    {"variant timeout", "-"},
		"al-toasts":   {"position", "-"},
		"al-tooltip":  {"for", "-"},
		"al-devpanel": {"open!", "-"},
	}
	m := Builtin()
	if len(m.Elements) != len(want) {
		t.Errorf("%d elements, want %d", len(m.Elements), len(want))
	}
	for tag, w := range want {
		el := m.Lookup(tag)
		if el == nil {
			t.Errorf("%s missing", tag)
			continue
		}
		if !el.Builtin {
			t.Errorf("%s is not marked Builtin", tag)
		}
		var attrs, slots []string
		for _, a := range el.Attrs {
			n := a.Name
			if a.Boolean() {
				n += "!"
			}
			attrs = append(attrs, n)
			if a.Description == "" || strings.Contains(a.Description, "\n") {
				t.Errorf("%s %s: want a one-line description, got %q", tag, a.Name, a.Description)
			}
		}
		for _, sl := range el.Slots {
			s := sl.Name
			if s == "" {
				s = "-"
			}
			slots = append(slots, s)
		}
		if got := strings.Join(attrs, " "); got != w.attrs {
			t.Errorf("%s attributes %q, want %q", tag, got, w.attrs)
		}
		if got := strings.Join(slots, " "); got != w.slots {
			t.Errorf("%s slots %q, want %q", tag, got, w.slots)
		}
	}
	if d := m.Lookup("al-button").Attr("variant").Default; d != `"primary"` {
		t.Errorf("al-button variant default %q", d)
	}
}

func TestParse_Subset(t *testing.T) {
	m, err := Parse([]byte(`{"schemaVersion":"1.0.0","modules":[{"kind":"javascript-module","declarations":[
		{"kind":"function","name":"f"},
		{"kind":"class","name":"WaButton","tagName":"wa-button","customElement":true,
		 "attributes":[{"name":"loading","type":{"text":" boolean "}},{"name":"variant","type":{"text":"'a' | 'b'"},"default":"'a'"}],
		 "slots":[{"name":""},{"name":"start"}],"cssParts":[{"name":"base"}],"events":[{"name":"x"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Elements) != 1 {
		t.Fatalf("elements %v", m.Elements)
	}
	el := m.Lookup("wa-button")
	if el.Builtin || !el.Attr("loading").Boolean() || el.Attr("variant").Boolean() || el.Attr("variant").Default != "'a'" {
		t.Errorf("%+v", el)
	}
	if !el.HasSlot("") || !el.HasSlot("start") || el.HasSlot("end") || !slices.Equal(el.NamedSlots(), []string{"start"}) {
		t.Errorf("slots %v", el.Slots)
	}
}

func TestParse_Errors(t *testing.T) {
	for _, c := range []struct {
		src, want string
		line, col int
	}{
		{`{"schemaVersion":"1.0.0"}`, "no `modules`", 1, 1},
		{"{\n  \"modules\": [ , ]\n}", "invalid character ',' looking for beginning of value", 2, 16},
		{`[]`, "the file must be an object, not array", 1, 1},
		{`{"modules": {}}`, "`modules` must be an array, not object", 1, 13},
		{`{"modules": [{"declarations": [{"tagName": 3}]}]}`, "`modules.0.declarations.0.tagName` must be a string, not number", 1, 44},
	} {
		_, err := Parse([]byte(c.src))
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%s: err %v", c.src, err)
			continue
		}
		if e.Reason != c.want || e.Line != c.line || e.Col != c.col {
			t.Errorf("%s: %d:%d %q, want %d:%d %q", c.src, e.Line, e.Col, e.Reason, c.line, c.col, c.want)
		}
	}
}

func TestLoad_Absent(t *testing.T) {
	m, err := Load(fstest.MapFS{}, Path)
	if m != nil || err != nil {
		t.Fatalf("%v %v", m, err)
	}
	m, err = Load(fstest.MapFS{Path: {Data: []byte(`{"modules":[]}`)}}, Path)
	if m == nil || err != nil {
		t.Fatalf("%v %v", m, err)
	}
}

// Review Focus 1: an app entry for a built-in tag replaces it whole.
func TestMerge_AppWins(t *testing.T) {
	app, err := Parse([]byte(`{"modules":[{"declarations":[{"tagName":"al-button","attributes":[{"name":"tone"}]},{"tagName":"wa-icon"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	b := Builtin()
	m := Merge(b, app)
	btn := m.Lookup("al-button")
	if btn.Builtin || btn.Attr("variant") != nil || btn.Attr("tone") == nil {
		t.Errorf("al-button %+v", btn)
	}
	if m.Lookup("wa-icon") == nil || !m.Lookup("al-card").Builtin {
		t.Error("union lost an element")
	}
	if !b.Lookup("al-button").Builtin || b.Lookup("al-button").Attr("variant") == nil {
		t.Error("Merge changed the built-in manifest")
	}
	if Merge(b, nil).Lookup("al-page") == nil {
		t.Error("Merge with no app manifest")
	}
}

func TestEscaped(t *testing.T) {
	for _, n := range []string{"id", "class", "style", "slot", "hidden", "title", "lang", "dir", "tabindex", "role", "data-x", "aria-label", "click", "href", "popover", "popovertarget", "popovertargetaction", "commandfor", "command", "closedby"} {
		if !Escaped(n) {
			t.Errorf("%s not escaped", n)
		}
	}
	for _, n := range []string{"name", "value", "field", "label", "type", "disabled", "onclick", "data", "aria", "src", "submit", "anchor", "popovers"} {
		if Escaped(n) {
			t.Errorf("%s escaped", n)
		}
	}
}

func TestAttrValues(t *testing.T) {
	for typ, want := range map[string]string{
		`"sm" | "md" | "lg"`: "sm,md,lg",
		`"primary"`:          "primary",
		` 'a'|'b' `:          "a,b",
		`string`:             "",
		`"sm" | string`:      "",
		`number`:             "",
		`boolean`:            "",
		`"a" | "b`:           "",
		``:                   "",
	} {
		if got := strings.Join(Attr{Type: typ}.Values(), ","); got != want {
			t.Errorf("Values(%q) = %q, want %q", typ, got, want)
		}
	}
}

// Every built-in element and its default slot are described (the editor's hover text).
func TestBuiltin_Descriptions(t *testing.T) {
	for tag, el := range Builtin().Elements {
		if el.Description == "" {
			t.Errorf("%s has no description", tag)
		}
	}
	var def *Slot
	for i, s := range Builtin().Lookup("al-button").Slots {
		if s.Name == "" {
			def = &Builtin().Lookup("al-button").Slots[i]
		}
	}
	if def == nil || def.Description == "" {
		t.Errorf("al-button default slot %+v, want a description", def)
	}
}

// An app manifest without descriptions parses with empty ones.
func TestParse_NoDescriptions(t *testing.T) {
	m, err := Parse([]byte(`{"modules":[{"declarations":[{"tagName":"x-a","attributes":[{"name":"n"}],"slots":[{"name":""},{"name":"s"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	el := m.Lookup("x-a")
	if el.Description != "" || el.Attr("n").Description != "" || len(el.Slots) != 2 || el.Slots[0].Description != "" || el.Slots[1].Name != "s" {
		t.Errorf("%+v", el)
	}
}
