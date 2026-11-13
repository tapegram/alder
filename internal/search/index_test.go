package search

import (
	"strings"
	"testing"

	"alder/internal/buildkit"
)

func find(ix *Index, module, name string, kind Kind) *Entry {
	for i := range ix.Entries {
		e := &ix.Entries[i]
		if e.Module == module && e.Name == name && e.Kind == kind {
			return e
		}
	}
	return nil
}

func std(t *testing.T) *Index {
	t.Helper()
	ix, err := Std()
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestStd_Probe(t *testing.T) {
	ix := std(t)
	if len(ix.Entries) <= 300 {
		t.Fatalf("%d entries, want more than 300", len(ix.Entries))
	}
	for _, e := range ix.Entries {
		if e.Source != "stdlib" || e.File != "" {
			t.Fatalf("%s.%s: source %q file %q", e.Module, e.Name, e.Source, e.File)
		}
	}
}

func TestStd_Entries(t *testing.T) {
	ix := std(t)
	for _, c := range []struct {
		module, name string
		kind         Kind
		sig          string
	}{
		{"Text", "split", Function, "Text -> Text -> List Text"},
		{"Prelude", "Just", Constructor, "a -> Maybe a"},
		{"Prelude", "Nothing", Constructor, "Maybe a"},
		{"Prelude", "Maybe", TypeDecl, "type Maybe a"},
		{"Text", "Builder", TypeDecl, ""},
	} {
		e := find(ix, c.module, c.name, c.kind)
		if e == nil {
			t.Errorf("%s.%s (%s) not found", c.module, c.name, c.kind)
			continue
		}
		if c.sig != "" && e.Signature != c.sig {
			t.Errorf("%s.%s: signature %q, want %q", c.module, c.name, e.Signature, c.sig)
		}
		if (e.Kind == TypeDecl) != (e.Type == nil) {
			t.Errorf("%s.%s: Type %v for kind %s", c.module, c.name, e.Type, e.Kind)
		}
	}
	if e := find(ix, "Text", "left", Function); e == nil || e.Doc != "the first n code points" {
		t.Errorf("Text.left: %+v", e)
	}
	if e := find(ix, "Query", "delay", Function); e == nil || e.Effect != "query" {
		t.Errorf("Query.delay: %+v", e)
	}
	pos := map[string]int{}
	for i, e := range ix.Entries {
		if e.Module == "Text" {
			if _, seen := pos[e.Name]; !seen {
				pos[e.Name] = i
			}
		}
	}
	if !(pos["length"] < pos["isEmpty"] && pos["isEmpty"] < pos["trim"]) {
		t.Errorf("Text order: length %d isEmpty %d trim %d", pos["length"], pos["isEmpty"], pos["trim"])
	}
}

func TestBuild_App(t *testing.T) {
	prog, res, _, errs := buildkit.LoadChecked("../../examples/blog", nil)
	if len(errs) > 0 || res == nil {
		t.Fatalf("load: %v", errs)
	}
	ix := Build(prog, res)
	app, stdlib := false, false
	for _, e := range ix.Entries {
		if strings.Contains(e.Name, ".") {
			t.Errorf("route member %q indexed", e.Name)
		}
		if e.Source == "app" && strings.HasPrefix(e.File, "app/") && e.Line > 0 {
			app = true
		}
		if e.Source == "stdlib" {
			stdlib = true
		}
	}
	if !app || !stdlib {
		t.Errorf("app entries %v, stdlib entries %v", app, stdlib)
	}
}

func TestBuild_SkipsUntyped(t *testing.T) {
	prog, res, _, errs := buildkit.LoadChecked("../../testdata/corpus/E0002-own-module", nil)
	if len(errs) > 0 || res == nil {
		t.Fatalf("load: %v", errs)
	}
	for _, e := range Build(prog, res).Entries {
		if (e.Kind == Function || e.Kind == Value) && e.Type == nil {
			t.Errorf("%s.%s has no type", e.Module, e.Name)
		}
	}
}
