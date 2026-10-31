package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const idx = "app/Posts/Index.alder"

// completeTyped opens the blog's Posts.Index, types typed on a line of its own after the `q`
// binding, and completes at its end.
func completeTyped(t *testing.T, root, typed string) map[string]item {
	t.Helper()
	c := start(t, root)
	openClean(t, c, idx)
	p := typing(t, c, idx, afterQ, typed)
	return labels(complete(t, c, idx, p))
}

func writeManifest(t *testing.T, root, json string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "elements"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "elements", "custom-elements.json"), []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestComplete_ElementTags(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <al")
	btn, ok := got["al-button"]
	if !ok {
		t.Fatalf("no al-button: %v", got)
	}
	if btn.Kind != 7 || btn.Detail != "al-button" || btn.Documentation == nil || btn.Documentation.Kind != "markdown" || btn.Documentation.Value == "" {
		t.Fatalf("al-button: %+v", btn)
	}
	if _, ok := got["al-devpanel"]; !ok {
		t.Fatalf("no al-devpanel: %v", got)
	}
	if _, ok := got["div"]; !ok {
		t.Fatalf("HTML tags dropped")
	}
}

func TestComplete_ElementAttrs(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <al-button v")
	v, ok := got["variant"]
	if !ok {
		t.Fatalf("no variant: %v", got)
	}
	if !strings.HasPrefix(v.Detail, `"primary" |`) || !strings.HasSuffix(v.Detail, `= "primary"`) || v.TextEdit.NewText != `variant=""` || v.Kind != 10 {
		t.Fatalf("variant: %+v", v)
	}
	if v.Documentation == nil || v.Documentation.Value == "" {
		t.Fatalf("variant documentation: %+v", v)
	}
	if _, ok := got["class"]; !ok {
		t.Fatalf("global attributes dropped")
	}
	if _, ok := got["slot"]; ok {
		t.Fatalf("slot offered outside a custom element")
	}
}

func TestComplete_AttrEnumValues(t *testing.T) {
	got := completeTyped(t, blogCopy(t), `  let z = <al-button variant="`)
	for _, v := range []string{"primary", "secondary", "destructive", "outline", "ghost", "link"} {
		it, ok := got[v]
		if !ok || it.Kind != 20 || it.TextEdit.NewText != v {
			t.Fatalf("%s: %+v in %v", v, it, got)
		}
	}
	if len(got) != 6 {
		t.Fatalf("want the six variants: %v", got)
	}
	got = completeTyped(t, blogCopy(t), `  let z = <al-button variant="gh`)
	it := got["ghost"]
	if it.TextEdit.Range.End.Character-it.TextEdit.Range.Start.Character != 2 {
		t.Fatalf("ghost replaces the typed prefix: %+v", it)
	}
}

func TestComplete_BooleanAttrBare(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <al-button disabl")
	if it, ok := got["disabled"]; !ok || it.TextEdit.NewText != "disabled" {
		t.Fatalf("disabled: %+v", it)
	}
}

func TestComplete_InputAttrsNotButtons(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <al-input l")
	if _, ok := got["label"]; !ok {
		t.Fatalf("no label: %v", got)
	}
	if _, ok := got["variant"]; ok {
		t.Fatalf("variant on al-input")
	}
}

func TestComplete_SlotNames(t *testing.T) {
	got := completeTyped(t, blogCopy(t), `  let z = <al-card><p slot="`)
	if len(got) == 0 {
		t.Fatal("no slots for al-card")
	}
	for name, it := range got {
		if it.Kind != 20 || name == "" {
			t.Fatalf("slot %q: %+v", name, it)
		}
	}
	if got := completeTyped(t, blogCopy(t), `  let z = <div><p slot="`); len(got) != 0 {
		t.Fatalf("div has no slots: %v", got)
	}
}

func TestComplete_SlotAttrInCustomParent(t *testing.T) {
	got := completeTyped(t, blogCopy(t), `  let z = <al-card><p s`)
	if _, ok := got["slot"]; !ok {
		t.Fatalf("slot not offered under al-card: %v", got)
	}
}

func TestComplete_AppManifestOverride(t *testing.T) {
	root := blogCopy(t)
	writeManifest(t, root, `{"modules":[{"declarations":[
		{"tagName":"x-chart","description":"A chart."},
		{"tagName":"al-badge","description":"Overridden badge."}]}]}`)
	got := completeTyped(t, root, "  let z = <x-ch")
	if it, ok := got["x-chart"]; !ok || it.Documentation == nil || !strings.Contains(it.Documentation.Value, "A chart.") {
		t.Fatalf("x-chart: %+v", it)
	}
	got = completeTyped(t, root, "  let z = <al-ba")
	if it := got["al-badge"]; it.Documentation == nil || !strings.Contains(it.Documentation.Value, "Overridden badge.") {
		t.Fatalf("al-badge: %+v", it)
	}
}

func TestComplete_UnparsedFileStillAl(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(idx, "module Posts.Index\n\nlet z = (((\n  <al-")
	p := Position{Line: 3, Character: len("  <al-")}
	if _, ok := labels(complete(t, c, idx, p))["al-button"]; !ok {
		t.Fatal("al-button missing in a file that does not parse")
	}
}

func TestComplete_AttrValueAcrossLines(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <al-button\n    variant=\"")
	if _, ok := got["ghost"]; !ok {
		t.Fatalf("variants across lines: %v", got)
	}
	got = completeTyped(t, blogCopy(t), "  let z = <al-button\n    v")
	if _, ok := got["variant"]; !ok {
		t.Fatalf("attrs across lines: %v", got)
	}
}

func TestComplete_AttrsOfInnermostTag(t *testing.T) {
	got := completeTyped(t, blogCopy(t), `  let z = <al-button variant="ghost">x</al-button> <p v`)
	if _, ok := got["variant"]; ok {
		t.Fatalf("al-button attrs on p: %v", got)
	}
}

func TestComplete_UnknownCustomTag(t *testing.T) {
	got := completeTyped(t, blogCopy(t), "  let z = <x-unknown v")
	if _, ok := got["class"]; !ok {
		t.Fatalf("no global attrs: %v", got)
	}
	if _, ok := got["variant"]; ok {
		t.Fatalf("al-* attrs on x-unknown")
	}
}

func TestComplete_SlotDirectParentOnly(t *testing.T) {
	if got := completeTyped(t, blogCopy(t), `  let z = <al-card><div><p slot="`); len(got) != 0 {
		t.Fatalf("nested slot: %v", got)
	}
}

func TestComplete_BrokenAppManifest(t *testing.T) {
	root := blogCopy(t)
	writeManifest(t, root, `{"modules":[{"declarations":[{"tagName":"x-chart"}]}]}`)
	c := start(t, root)
	openClean(t, c, idx)
	writeManifest(t, root, `{not json`)
	p := typing(t, c, idx, afterQ, "  let z = <")
	got := labels(complete(t, c, idx, p))
	if _, ok := got["al-button"]; !ok {
		t.Fatalf("al-button missing: %v", got)
	}
	// The last good build's merged manifest still answers, so x-chart stays until a build passes.
}
