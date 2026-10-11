package buildkit

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/ir"
	"alder/rt/db"
)

// lowered checks and lowers the app in dir, failing on a diagnostic error.
func lowered(t *testing.T, dir string) *ir.Program {
	t.Helper()
	prog, res, bag, ioErrs := LoadChecked(dir, nil)
	if len(ioErrs) > 0 {
		t.Fatal(ioErrs)
	}
	if bag.HasErrors() || res == nil {
		t.Fatalf("%s does not check: %v", dir, bag.Items)
	}
	return ir.Lower(prog, res)
}

// copyApp copies the app in src (app/ and migrations/) to a temporary directory.
func copyApp(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	return dst
}

func edit(t *testing.T, path, old, new string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s lacks %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

const migrated = "../../testdata/golden/gen_migrations" // an app with migrations, in step with them

func TestSchemaDrift(t *testing.T) {
	// no migrations/: nothing to compare
	if d := SchemaDrift("../../testdata/golden/gen_effects", lowered(t, "../../testdata/golden/gen_effects")); d != nil {
		t.Fatalf("absent: %+v", d)
	}
	// in step
	if d := SchemaDrift(migrated, lowered(t, migrated)); d != nil {
		t.Fatalf("equal: %+v", d)
	}
	// a default changed: not in the tables, no migration needed
	dir := copyApp(t, migrated)
	edit(t, filepath.Join(dir, "app", "Tag.alder"), "label: Text,", `label: Text = "none",`)
	if d := SchemaDrift(dir, lowered(t, dir)); d != nil {
		t.Fatalf("default only: %+v", d)
	}

	cases := []struct{ name, file, old, new, want string }{
		{"column added", "Tag.alder", "label: Text,", "label: Text,\n  color: Maybe Text,", "table Tag: column color"},
		{"column changed", "Tag.alder", "label: Text,", "label: Int,", "table Tag: column label"},
		{"column removed", "Tag.alder", "  label: Text,\n", "", "table Tag: column label"},
		{"columns reordered", "Note.alder", "  title: Text,\n  created: Time @created,", "  created: Time @created,\n  title: Text,", "table Note: column order"},
		{"delete rule", "Tag.alder", "@cascade", "@restrict", "table Tag: column note"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := copyApp(t, migrated)
			edit(t, filepath.Join(dir, "app", c.file), c.old, c.new)
			d := SchemaDrift(dir, lowered(t, dir))
			want := "the schema changed since migration 0001 (" + c.want + "): run `alder migrate`"
			if d == nil || d.Message != want {
				t.Fatalf("got %+v, want %q", d, want)
			}
			if d.Code != "E0019" || d.Severity != "error" || d.Span.File != "migrations/schema.json" || d.Span.Start != (diag.Pos{Line: 1, Col: 1}) {
				t.Fatalf("diagnostic %+v", d)
			}
		})
	}

	// a table added
	dir = copyApp(t, migrated)
	if err := os.WriteFile(filepath.Join(dir, "app", "Other.alder"), []byte("module Other\n\ntype OtherId = brand Text\n\ntable Other {\n  id: OtherId @id,\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d := SchemaDrift(dir, lowered(t, dir)); d == nil || !strings.Contains(d.Message, "(table Other: new table)") {
		t.Fatalf("added: %+v", d)
	}
	// a table removed (the app keeps another)
	dir = copyApp(t, migrated)
	if err := os.Remove(filepath.Join(dir, "app", "Tag.alder")); err != nil {
		t.Fatal(err)
	}
	if d := SchemaDrift(dir, lowered(t, dir)); d == nil || !strings.Contains(d.Message, "(table Tag: removed)") {
		t.Fatalf("removed: %+v", d)
	}
	// the latest migration is named
	dir = copyApp(t, migrated)
	first, err := os.ReadFile(filepath.Join(dir, "migrations", "0001_initial.sql"))
	if err != nil {
		t.Fatal(err)
	}
	m1, err := db.ParseMigration("0001_initial", string(first))
	if err != nil {
		t.Fatal(err)
	}
	more := "-- alder migration 0002_more\n-- from " + m1.To + "\n-- to " + m1.To + "\n\nSELECT 1;\n"
	if err := os.WriteFile(filepath.Join(dir, "migrations", "0002_more.sql"), []byte(more), 0o644); err != nil {
		t.Fatal(err)
	}
	edit(t, filepath.Join(dir, "app", "Tag.alder"), "label: Text,", "label: Int,")
	if d := SchemaDrift(dir, lowered(t, dir)); d == nil || !strings.HasPrefix(d.Message, "the schema changed since migration 0002 (") {
		t.Fatalf("since: %+v", d)
	}
	// the files' chain: a file that does not start where the one before it ends (one is
	// missing), a last file that does not end at the snapshot, a malformed file
	for _, c := range []struct{ file, want string }{
		{"-- alder migration 0003_sub\n-- from 123456789abc\n-- to " + m1.To + "\n\nSELECT 1;\n",
			"migration 0003_sub does not follow 0001_initial: a file is missing or edited; regenerate it"},
		{"-- alder migration 0003_sub\n-- from " + m1.To + "\n-- to 123456789abc\n\nSELECT 1;\n",
			"migration 0003_sub does not lead to migrations/schema.json: a file is missing or edited; regenerate it"},
		{"-- alder migration 0003_sub\n-- from " + m1.To + "\n-- to " + m1.To + "\n\nSELECT 1;\nSELECT 2\n",
			"migration 0003_sub: line 6: a statement must end in `;`"},
	} {
		dir := copyApp(t, migrated)
		if err := os.WriteFile(filepath.Join(dir, "migrations", "0003_sub.sql"), []byte(c.file), 0o644); err != nil {
			t.Fatal(err)
		}
		if d := SchemaDrift(dir, lowered(t, dir)); d == nil || d.Message != c.want || d.Code != "E0019" {
			t.Fatalf("chain: %+v, want %q", d, c.want)
		}
	}
	// migration files without the snapshot, and a snapshot that does not read
	dir = copyApp(t, migrated)
	os.Remove(filepath.Join(dir, "migrations", "schema.json"))
	if d := SchemaDrift(dir, lowered(t, dir)); d == nil || !strings.Contains(d.Message, "no schema.json") {
		t.Fatalf("no snapshot: %+v", d)
	}
	os.WriteFile(filepath.Join(dir, "migrations", "schema.json"), []byte("{"), 0o644)
	if d := SchemaDrift(dir, lowered(t, dir)); d == nil || !strings.Contains(d.Message, "migrations/schema.json does not read") {
		t.Fatalf("bad snapshot: %+v", d)
	}
}

func TestCopyMigrations(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "migrations")
	if err := CopyMigrations(filepath.Join(migrated, "migrations"), dst); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dst)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"0001_initial.sql"}) {
		t.Fatalf("copied %v", names)
	}
	// an app without migrations: the earlier copy is removed
	if err := CopyMigrations(filepath.Join(t.TempDir(), "migrations"), dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("stale copy kept: %v", err)
	}
	// schema.json and no migration
	src := filepath.Join(t.TempDir(), "migrations")
	os.MkdirAll(src, 0o755)
	os.WriteFile(filepath.Join(src, "schema.json"), []byte("{}"), 0o644)
	if err := CopyMigrations(src, dst); err == nil || !strings.Contains(err.Error(), "no migration") {
		t.Fatalf("err = %v", err)
	}
	if !HasMigrations(migrated) || HasMigrations("../../testdata/golden/gen_effects") {
		t.Fatal("HasMigrations")
	}
}

// TestDriftCorpus runs the apps of testdata/drift as `alder check` checks them (the checker,
// then AddSchemaDrift) against their expected.diag. (The checker's own corpus, testdata/corpus,
// runs the checker alone, so an E0019 case lives here.)
func TestDriftCorpus(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/drift/*")
	if len(dirs) == 0 {
		t.Fatal("no drift cases")
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			prog, res, bag, ioErrs := LoadChecked(dir, nil)
			if len(ioErrs) > 0 {
				t.Fatal(ioErrs)
			}
			AddSchemaDrift(dir, prog, res, bag)
			var sb strings.Builder
			for _, d := range bag.Items {
				if d.Severity == "error" {
					fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
				}
			}
			want, err := os.ReadFile(filepath.Join(dir, "expected.diag"))
			if err != nil {
				t.Fatal(err)
			}
			if sb.String() != string(want) {
				t.Fatalf("got:\n%s\nwant:\n%s", sb.String(), want)
			}
		})
	}
}
