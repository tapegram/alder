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

// MigrationsChain reads dir/migrations: the snapshot schema.json (with its hash) and the
// migration files, and checks that they agree with themselves: every file parses
// (db.ParseMigration: "migration 0002_x: line 7: a statement must end in `;`"), no two share a
// number, each file's `-- from` is the previous file's `-- to`, and the last `-- to` is the
// snapshot's hash. ok is false when there is no schema.json and no file (an app without
// migrations); err is the first disagreement, a sentence naming the remedy. Both SchemaDrift
// (E0019) and `alder migrate` (which must not plan on top of a broken chain) use it.
func MigrationsChain(dir string) (snap migrate.Schema, snapHash string, ok bool, err error) {
	files := migrationFiles(dir)
	if !HasMigrations(dir) {
		if len(files) == 0 {
			return snap, "", false, nil
		}
		return snap, "", true, fmt.Errorf("migrations/ has migration files but no schema.json, the schema they lead to: restore it from version control")
	}
	snap, err = migrate.ReadSnapshot(filepath.Join(dir, snapshotFile))
	if err == nil {
		snapHash, err = snap.Hash()
	}
	if err != nil {
		return snap, "", true, fmt.Errorf("migrations/schema.json does not read (%v): restore it from version control", err)
	}
	// two files with one number: two branches each added the next migration
	byNum := map[string][]string{}
	for _, f := range files {
		num, _, _ := strings.Cut(filepath.Base(f), "_")
		byNum[num] = append(byNum[num], filepath.Base(f))
	}
	for _, f := range files {
		num, _, _ := strings.Cut(filepath.Base(f), "_")
		if same := byNum[num]; len(same) > 1 {
			return snap, snapHash, true, fmt.Errorf("two migrations are numbered %s (likely two branches: %s): keep the one already merged or deployed, delete the other, restore schema.json to its result, run `alder migrate`", num, strings.Join(same, ", "))
		}
	}
	const restore = "restore migrations/ from version control (`git checkout migrations/`)" // as SnapshotMismatch says
	var prev db.Migration
	for i, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return snap, snapHash, true, fmt.Errorf("migration %s does not read: %v", filepath.Base(f), err)
		}
		m, err := db.ParseMigration(strings.TrimSuffix(filepath.Base(f), ".sql"), string(b))
		if err != nil {
			return snap, snapHash, true, err
		}
		if i > 0 && m.From != prev.To {
			return snap, snapHash, true, fmt.Errorf("migration %s does not follow %s: a file is missing or edited; %s", m.Name, prev.Name, restore)
		}
		prev = m
	}
	if len(files) > 0 && prev.To != snapHash[:db.HashLen] {
		return snap, snapHash, true, &SnapshotMismatch{Last: prev.Name}
	}
	return snap, snapHash, true, nil
}

// SnapshotMismatch is MigrationsChain's error when the last migration file does not lead to
// schema.json (`alder migrate` says it its own way: it can also write the file again).
type SnapshotMismatch struct{ Last string }

func (e *SnapshotMismatch) Error() string {
	return "migration " + e.Last + " does not lead to migrations/schema.json: a file is missing or edited; restore migrations/ from version control (`git checkout migrations/`)"
}

// SchemaDrift compares the checked program's schema with dir/migrations/schema.json; nil when
// absent or equal; else an E0019 at migrations/schema.json:1:1 naming the first changed table:
// "the schema changed since migration 0003 (table Post: column body): run `alder migrate`".
// Schemas are equal when their hashes are (the tables' CREATE TABLE statements, what a
// database holds), so a changed default alone is no change. Before that it checks migrations/
// itself (MigrationsChain), whose every disagreement is an E0019 too. A changed storage format
// says it needs a rebuild of every table, which `alder migrate` cannot write yet.
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
	old, oldHash, ok, err := MigrationsChain(dir)
	if err != nil {
		return at("%v", err)
	}
	if !ok {
		return nil
	}
	cur := migrate.FromProgram(p)
	if old.Format != cur.Format {
		return at("%s", migrate.FormatChanged(old.Format, cur.Format))
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
// lowering that panics is a compiler bug: the panic surfaces (the command reports an internal
// error), as one in the checker does.
func AddSchemaDrift(dir string, prog *names.Program, res *check.Result, bag *diag.Bag) {
	if res == nil || bag.HasErrors() {
		return
	}
	if !HasMigrations(dir) && len(migrationFiles(dir)) == 0 {
		return
	}
	if d := SchemaDrift(dir, ir.Lower(prog, res)); d != nil {
		bag.Items = append(bag.Items, *d)
		bag.Sort()
	}
}
