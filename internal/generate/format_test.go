package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/format"
)

// TestRegionEnd_SameAsFormat: the formatter keeps an end marker with the item before it by its
// text, which it holds in a constant of its own (it cannot import this package).
func TestRegionEnd_SameAsFormat(t *testing.T) {
	if format.RegionEnd != RegionEnd {
		t.Fatalf("format.RegionEnd = %q, generate.RegionEnd = %q", format.RegionEnd, RegionEnd)
	}
}

func TestFormatted(t *testing.T) {
	out, w := Formatted("app/A.alder", "module A\n\n\n\nx : Int\nx =   1   \n\n")
	if out != "module A\n\nx : Int\nx = 1\n" || w != "" {
		t.Errorf("Formatted = %q, %q", out, w)
	}
	broken := "module A\n\nx = (\n"
	out, w = Formatted("app/A.alder", broken)
	if out != broken || !strings.HasPrefix(w, "output not formatted: app/A.alder:") || !strings.HasSuffix(w, "; please file an issue") {
		t.Errorf("Formatted of a parse error = %q, %q", out, w)
	}
}

// unformatted is the scaffold's Home with its layout undone: the declarations run together and
// a pipeline on one line, which `alder fmt` breaks.
func unformatted(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), "  let entries <-\n    Db.from Entry\n      |> Db.orderBy Entry.created.desc\n      |> Db.all\n", "  let entries <- Db.from Entry |> Db.orderBy Entry.created.desc |> Db.all\n", 1)
	s = strings.ReplaceAll(s, "\n\n", "\n\n\n")
	if s == string(b) {
		t.Fatal("the scaffold's Home has changed: update unformatted")
	}
	return s
}

// TestSplice_FormatsWholeFile: an edit to a file that is not formatted writes the whole file
// formatted, not only the part the generator changed (spec §2).
func TestSplice_FormatsWholeFile(t *testing.T) {
	dir := scaffoldApp(t, nil)
	src := unformatted(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "app", "Home.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Apply(dir, []Edit{{Path: "app/Home.alder", Body: src + "\nanswer : Int\nanswer =   42\n"}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) > 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	want, _, err := format.Source("app/Home.alder", b)
	if err != nil || string(want) != string(b) {
		t.Fatalf("the file written is not formatted (%v):\n%s", err, b)
	}
	if !strings.HasSuffix(string(b), "\nanswer : Int\nanswer = 42\n") || strings.Contains(string(b), "<- Db.from Entry |>") {
		t.Fatalf("the edit, or the rest of the file, is not formatted:\n%s", b)
	}
}

// TestSplice_UnparsableFileWarns: a file that did not parse before the edit is written as the
// generator made it, with the warning; nothing else is lost.
func TestSplice_UnparsableFileWarns(t *testing.T) {
	broken := "module Note\n\nnote : Int\nnote = (\n"
	dir := scaffoldApp(t, map[string]string{"app/Note.alder": broken})
	body := broken + "\n\n\n-- the generator's\n"
	res, err := Apply(dir, []Edit{{Path: "app/Note.alder", Body: body}}, Options{WriteDespiteErrors: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "output not formatted: `app/Note.alder` did not parse before the edit; please file an issue"
	if len(res.Warnings) != 1 || res.Warnings[0] != want {
		t.Errorf("warnings %q, want [%q]", res.Warnings, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Note.alder")); string(b) != body {
		t.Errorf("written %q, want the body unformatted", b)
	}
}

// TestCreate_Formatted: a file a generator creates is formatted, and one that cannot be is
// written as it is, with the warning.
func TestCreate_Formatted(t *testing.T) {
	dir := scaffoldApp(t, nil)
	res, err := Apply(dir, []Edit{{Path: "app/Note.alder", Create: true, Body: "module Note\n\n\nnote : Int\nnote =   1\n\n"}}, Options{})
	if err != nil || len(res.Warnings) > 0 {
		t.Fatalf("%v %v", err, res)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Note.alder")); string(b) != "module Note\n\nnote : Int\nnote = 1\n" {
		t.Errorf("created %q", b)
	}
	broken := "module Bad\n\nbad = (\n"
	res, _ = Apply(dir, []Edit{{Path: "app/Bad.alder", Create: true, Body: broken}}, Options{DryRun: true})
	if res == nil || len(res.Warnings) != 1 || !strings.HasPrefix(res.Warnings[0], "output not formatted: app/Bad.alder:4:") {
		t.Fatalf("warnings of a created file that does not parse: %+v", res)
	}
	for _, f := range res.Files {
		if f.Path == "app/Bad.alder" && *f.After != broken {
			t.Errorf("created %q, want it unformatted", *f.After)
		}
	}
}
