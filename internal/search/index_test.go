package search

import (
	"slices"
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
	if e := find(ix, "Text", "left", Function); e == nil || e.Doc == "" {
		t.Errorf("Text.left: %+v", e)
	}
	if e := find(ix, "List", "take", Function); e == nil || e.Summary != "The first n elements of a list." ||
		len(e.Examples) == 0 || e.Examples[0] != "List.take 2 [1, 2, 3] == [1, 2]" || e.Book != `chapter 9, "List"` {
		t.Errorf("List.take: %+v", e)
	}
	// No book line of its own: the List module's (I-3).
	if e := find(ix, "List", "drop", Function); e == nil || strings.Contains(e.Doc, "Book:") || e.Book != `chapter 9, "List"` {
		t.Errorf("List.drop: %+v", e)
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

// A job is a declaration search finds, with its constructor's type, and so is its `run` arm
// (a route's members are not: a job has only the one).
func TestBuild_Job(t *testing.T) {
	prog, res, _, errs := buildkit.LoadChecked("../../examples/todos", nil)
	if len(errs) > 0 || res == nil {
		t.Fatalf("load: %v", errs)
	}
	ix := Build(prog, res)
	job := find(ix, "Jobs.Nightly", "Nightly", Job)
	if job == nil || job.Signature != "Jobs.Nightly.Nightly.Params -> Job" || job.File != "app/Jobs/Nightly.alder" || job.Line != 3 || job.Col != 1 {
		t.Fatalf("job Nightly: %+v", job)
	}
	run := find(ix, "Jobs.Nightly", "Nightly.run", Function)
	if run == nil || run.Signature != "Jobs.Nightly.Nightly.Params -> Task Unit" || run.Effect != "task" || run.Line != 5 {
		t.Fatalf("Nightly.run: %+v", run)
	}
	var names []string
	for _, h := range ix.Search("Nightly", Options{Source: "app", Limit: -1}).Results {
		names = append(names, h.Entry.Name)
	}
	if !slices.Contains(names, "Nightly") || !slices.Contains(names, "Nightly.run") {
		t.Errorf("search Nightly: %v", names)
	}
}
