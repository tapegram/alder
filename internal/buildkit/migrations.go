package buildkit

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/migrate"
	"alder/internal/names"
	"alder/rt/db"
)

// snapshotFile is where a diagnostic about the app's migrations points: dir's
// migrations/schema.json, as a path relative to the app (like "app/Post.alder").
const snapshotFile = "migrations/schema.json"

// HasMigrations reports whether the app in dir has migrations: dir/migrations/schema.json, the
// snapshot `alder migrate` writes with every migration, exists.
func HasMigrations(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, snapshotFile))
	return err == nil && !st.IsDir()
}

// migrationFiles are the migrations/*.sql files of the app in dir, sorted (nil without any).
func migrationFiles(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
	slices.Sort(files)
	return files
}

// CopyMigrations replaces the directory dst with the *.sql files of the directory src (an
// app's migrations/, copied into the generated module, whose app__alder.go embeds them when
// HasMigrations), or removes dst when src has no schema.json (an app without migrations). A src
// with schema.json but no migration file is an error: there would be nothing to embed.
func CopyMigrations(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if st, err := os.Stat(filepath.Join(src, "schema.json")); err != nil || st.IsDir() {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(src, "*.sql"))
	slices.Sort(files)
	if len(files) == 0 {
		return fmt.Errorf("%s has schema.json but no migration (*.sql): run `alder migrate`", src)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.Base(f)), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// SchemaDrift compares the checked program's schema with dir/migrations/schema.json; nil when
// absent or equal; else an E0019 at migrations/schema.json:1:1 naming the first changed table:
// "the schema changed since migration 0003 (table Post: column body): run `alder migrate`".
// Schemas are equal when their hashes are (the tables' CREATE TABLE statements, what a
// database holds), so a changed default alone is no change. Migration files without a
// schema.json, and a schema.json that does not read, are E0019 too.
func SchemaDrift(dir string, p *ir.Program) *diag.Diagnostic {
	files := migrationFiles(dir)
	since := "the last migration"
	if len(files) > 0 {
		base := filepath.Base(files[len(files)-1])
		num, _, _ := strings.Cut(base, "_")
		since = "migration " + num
	}
	at := func(format string, args ...any) *diag.Diagnostic {
		return &diag.Diagnostic{Code: diag.SchemaChanged, Severity: "error",
			Span:    diag.Span{File: snapshotFile, Start: diag.Pos{Line: 1, Col: 1}, End: diag.Pos{Line: 1, Col: 1}},
			Message: fmt.Sprintf(format, args...)}
	}
	if !HasMigrations(dir) {
		if len(files) == 0 {
			return nil
		}
		return at("migrations/ has migration files but no schema.json, the schema they lead to: restore it from version control")
	}
	old, err := migrate.ReadSnapshot(filepath.Join(dir, snapshotFile))
	var oldHash string
	if err == nil {
		oldHash, err = old.Hash()
	}
	if err != nil {
		return at("migrations/schema.json does not read (%v): restore it from version control", err)
	}
	cur := migrate.FromProgram(p)
	if old.Format != cur.Format {
		return at("the storage format changed since %s (v%s to v%s): run `alder migrate`", since, old.Format, cur.Format)
	}
	curHash, err := cur.Hash()
	if err != nil {
		return at("the schema changed since %s: run `alder migrate` (%v)", since, err)
	}
	if oldHash == curHash {
		return nil
	}
	return at("the schema changed since %s (%s): run `alder migrate`", since, firstChange(old, cur))
}

// firstChange describes the first table (by name) whose CREATE TABLE statement differs between
// the two schemas: "table Post: column body", "table Post: new table", "table Post: removed".
func firstChange(old, cur migrate.Schema) string {
	stmts := func(s migrate.Schema) map[string]string {
		m := map[string]string{}
		sts, _ := s.CreateStmts()
		for i, t := range s.Tables {
			if i < len(sts) {
				m[t.Name] = sts[i]
			}
		}
		return m
	}
	tables := func(s migrate.Schema) map[string]migrate.Table {
		m := map[string]migrate.Table{}
		for _, t := range s.Tables {
			m[t.Name] = t
		}
		return m
	}
	oldStmts, curStmts := stmts(old), stmts(cur)
	ot, ct := tables(old), tables(cur)
	var all []string
	for n := range ot {
		all = append(all, n)
	}
	for n := range ct {
		if _, ok := ot[n]; !ok {
			all = append(all, n)
		}
	}
	slices.Sort(all)
	for _, n := range all {
		label := "table " + n[strings.LastIndex(n, ".")+1:]
		o, inOld := ot[n]
		c, inCur := ct[n]
		switch {
		case !inOld:
			return label + ": new table"
		case !inCur:
			return label + ": removed"
		case oldStmts[n] == curStmts[n]:
			continue
		}
		od, cd := o.Desc(), c.Desc()
		for _, cc := range cd.Columns {
			i := slices.IndexFunc(od.Columns, func(x db.Column) bool { return x.Name == cc.Name })
			if i < 0 || !reflect.DeepEqual(od.Columns[i], cc) {
				return label + ": column " + cc.Name
			}
		}
		for _, oc := range od.Columns {
			if !slices.ContainsFunc(cd.Columns, func(x db.Column) bool { return x.Name == oc.Name }) {
				return label + ": column " + oc.Name
			}
		}
		if len(od.Columns) == len(cd.Columns) {
			return label + ": column order"
		}
		return label
	}
	return "its tables"
}

// AddSchemaDrift adds SchemaDrift's E0019 for the app in dir to bag (sorted again) when the
// program checked without errors (res non-nil): the program is lowered to read its tables. A
// lowering that fails is left for the build to report.
func AddSchemaDrift(dir string, prog *names.Program, res *check.Result, bag *diag.Bag) {
	if res == nil || bag.HasErrors() {
		return
	}
	if !HasMigrations(dir) && len(migrationFiles(dir)) == 0 {
		return
	}
	var d *diag.Diagnostic
	func() {
		defer func() { recover() }()
		d = SchemaDrift(dir, ir.Lower(prog, res))
	}()
	if d != nil {
		bag.Items = append(bag.Items, *d)
		bag.Sort()
	}
}
