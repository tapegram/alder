package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"alder/internal/buildkit"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/migrate"
)

// migrateUsage is the usage line of `alder migrate`.
const migrateUsage = "usage: alder migrate [dir] [--name slug] [--dry-run]"

// maxSlug bounds a derived migration name (the part after "NNNN_").
const maxSlug = 40

// runMigrateTo is `alder migrate [dir] [--name slug] [--dry-run]`: it checks the app at dir
// (default "."), diffs its tables against dir/migrations/schema.json (the schema the last
// migration leads to; an empty schema when there is none yet) and writes the next migration,
// migrations/NNNN_<slug>.sql, and the new schema.json. The slug is --name, or derived from the
// steps ("initial" for the first migration). Each step is printed, a step that loses data
// followed by a `!! DROPS DATA: …` line. With --dry-run the file is printed and nothing is
// written. No change since the last migration prints "no changes since NNNN_x" and writes
// nothing (but a snapshot whose defaults alone changed is rewritten).
//
// The check here never reports E0019 (the schema changed since the last migration): that is
// what this command fixes. Exit 0 on success or no change; 1 for check errors, a change Diff
// refuses (a new non-null column without a default, a `@renamed` that names nothing, …), or
// migrations/ that do not agree with themselves (a schema.json that is not the last file's
// result, a file that does not follow the one before it, two files with one number:
// buildkit.MigrationsChain); 2 for usage errors and files that cannot be read or written.
func runMigrateTo(args []string, stdout, stderr io.Writer) int {
	return runMigrateWith(args, stdout, stderr, nil)
}

// runMigrateWith is runMigrateTo with the deploy commands' environment of `alder migrate silos`
// (nil: osDeployEnv's).
func runMigrateWith(args []string, stdout, stderr io.Writer, e *deployEnv) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr, "alder migrate: internal error: %v\n", r)
			code = 2
		}
	}()
	if len(args) >= 2 && args[0] == "silos" && !strings.HasPrefix(args[1], "-") {
		if e == nil {
			return runMigrateSilosTo(args[1:], stdout, stderr) // `alder migrate silos <env>`
		}
		return runMigrateSilosWith(args[1:], stdout, stderr, *e)
	}
	dir, name, dry, haveDir := ".", "", false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--dry-run":
			dry = true
		case a == "--name" && i+1 < len(args):
			i++
			name = args[i]
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case strings.HasPrefix(a, "-") || haveDir:
			fmt.Fprintln(stderr, migrateUsage)
			return 2
		default:
			dir, haveDir = a, true
		}
	}

	prog, res, bag, ioErrs := buildkit.LoadChecked(dir, nil) // no SchemaDrift: this is its fix
	if reportLoadErrs("alder migrate", ioErrs, stderr) {
		return 2
	}
	if len(prog.Modules) == 0 {
		fmt.Fprintf(stderr, "alder migrate: no .alder files under %s/app\n", dir)
		return 2
	}
	sources := buildkit.Sources(prog)
	for _, d := range bag.Items {
		w := stdout
		if d.Severity == "warning" {
			w = stderr
		}
		fmt.Fprint(w, diag.RenderText(d, sources[d.Span.File]))
	}
	if bag.HasErrors() || res == nil {
		return 1
	}
	return writeMigration(dir, migrate.FromProgram(ir.Lower(prog, res)), name, dry, stdout, stderr)
}

// writeMigration is the second half of runMigrateTo: it plans the migration from the schema of
// dir/migrations to cur, the app's tables, and writes it (or, with dry, prints it), with
// runMigrateTo's output and exit codes. `alder new table` and `alder edit table` call it on
// their copy of the app (internal/generate's Options.Migrate), where cur comes from the
// tables alone (ir.LowerTables).
func writeMigration(dir string, cur migrate.Schema, name string, dry bool, stdout, stderr io.Writer) int {
	mdir := filepath.Join(dir, "migrations")
	snap := filepath.Join(mdir, "schema.json")
	old, last, code := previous(mdir, stderr)
	if code != 0 {
		return code
	}
	// the files must agree with themselves before a migration is planned on top of them (a
	// middle file deleted, two numbered alike): `alder check`'s E0019 texts
	if _, _, _, err := buildkit.MigrationsChain(dir); err != nil {
		var sm *buildkit.SnapshotMismatch
		if !errors.As(err, &sm) { // (previous has said that one its own way)
			fmt.Fprintf(stderr, "alder migrate: %v\n", err)
			return 1
		}
	}

	m, err := migrate.Plan(old, cur, "next")
	if err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			fmt.Fprintf(stdout, "alder migrate: %s\n", line)
		}
		return 1
	}
	if len(m.Steps) == 0 {
		switch {
		case last == "":
			fmt.Fprintln(stdout, "no tables: nothing to migrate")
		case dry:
			fmt.Fprintf(stdout, "no changes since %s\n", last)
		default:
			fmt.Fprintf(stdout, "no changes since %s\n", last)
			if changed, err := snapshotDiffers(snap, cur); err != nil {
				fmt.Fprintf(stderr, "alder migrate: %v\n", err)
				return 2
			} else if changed { // the defaults (not in the database) changed: keep schema.json current
				if err := migrate.WriteSnapshot(snap, cur); err != nil {
					fmt.Fprintf(stderr, "alder migrate: %v\n", err)
					return 2
				}
				fmt.Fprintf(stdout, "updated %s (defaults only)\n", snap)
			}
		}
		return 0
	}

	slug := name
	if slug == "" {
		slug = "initial"
		if last != "" {
			slug = deriveSlug(m.Steps)
		}
	}
	if m.Name, err = migrate.NextName(mdir, slug); err != nil {
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return 2
	}
	text := migrate.Render(m)
	file := filepath.Join(mdir, m.Name+".sql")
	if dry {
		fmt.Fprint(stdout, text)
		fmt.Fprintf(stdout, "\ndry run: %s not written\n", file)
		return 0
	}
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return 2
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return 2
	}
	if err := migrate.WriteSnapshot(snap, cur); err != nil {
		os.Remove(file)
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "wrote %s\n", file)
	for _, s := range m.Steps {
		fmt.Fprintf(stdout, "  %s\n", s)
		if s.DropsData != "" {
			fmt.Fprintf(stdout, "!! DROPS DATA: %s\n", s.DropsData)
		}
	}
	fmt.Fprintf(stdout, "updated %s\n", snap)
	return 0
}

var migrationFileRe = regexp.MustCompile(`^[0-9]{4}_.+\.sql$`)

// previous reads the migrations directory mdir: the schema its last migration leads to
// (schema.json) and that migration's name, or the empty schema and "" when there is no
// migration yet. A schema.json without migration files, files without a schema.json, and a
// schema.json that is not the last file's `-- to` schema are refused (exit 1).
func previous(mdir string, stderr io.Writer) (migrate.Schema, string, int) {
	entries, err := os.ReadDir(mdir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return migrate.Schema{}, "", 2
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && migrationFileRe.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	slices.Sort(files)
	snap := filepath.Join(mdir, "schema.json")
	_, serr := os.Stat(snap)
	switch {
	case len(files) == 0 && serr != nil:
		return migrate.Schema{}, "", 0
	case len(files) == 0:
		fmt.Fprintf(stderr, "alder migrate: migrations/schema.json has no migration file: restore the migrations from version control, or delete schema.json to start again from 0001_initial\n")
		return migrate.Schema{}, "", 1
	case serr != nil:
		fmt.Fprintf(stderr, "alder migrate: migrations/ has migration files but no schema.json, the schema they lead to: restore it from version control\n")
		return migrate.Schema{}, "", 1
	}
	lastFile := files[len(files)-1]
	last := strings.TrimSuffix(lastFile, ".sql")
	old, err := migrate.ReadSnapshot(snap)
	var hash string
	if err == nil {
		hash, err = old.Hash()
	}
	if err != nil {
		fmt.Fprintf(stderr, "alder migrate: migrations/schema.json does not read (%v): restore it from version control\n", err)
		return old, last, 1
	}
	b, err := os.ReadFile(filepath.Join(mdir, lastFile))
	if err != nil {
		fmt.Fprintf(stderr, "alder migrate: %v\n", err)
		return old, last, 2
	}
	_, _, to, _, err := migrate.ParseHeader(string(b))
	if err != nil {
		fmt.Fprintf(stderr, "alder migrate: migrations/%s: %v\n", lastFile, err)
		return old, last, 1
	}
	if !strings.HasPrefix(hash, to) {
		fmt.Fprintf(stderr, "alder migrate: migrations/schema.json does not match %s's result; restore migrations/schema.json from version control, or delete the last migration file and run `alder migrate` again (only if it has not run against a database you keep)\n", lastFile)
		return old, last, 1
	}
	return old, last, 0
}

// snapshotDiffers reports whether the snapshot file at path differs from what WriteSnapshot
// would write for s.
func snapshotDiffers(path string, s migrate.Schema) (bool, error) {
	old, err := migrate.ReadSnapshot(path)
	if err != nil {
		return false, err
	}
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(sortedSchema(s))
	return !bytes.Equal(a, b), nil
}

// sortedSchema is s with its tables in name order, as a snapshot holds them.
func sortedSchema(s migrate.Schema) migrate.Schema {
	s.Tables = slices.Clone(s.Tables)
	if s.Tables == nil {
		s.Tables = []migrate.Table{}
	}
	slices.SortFunc(s.Tables, func(a, b migrate.Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

// deriveSlug names a migration after its steps: create_<table>, drop_<table>, rename_<old name>,
// add_<column>, drop_<column>, change_<table> (a rebuild); index steps add nothing. The words
// are lower case with `_` between words ("publishedAt" is published_at); repeated parts appear
// once; parts are joined with `_` while the whole stays within 40 characters ("change" when no
// step names anything).
func deriveSlug(steps []migrate.Step) string {
	var parts []string
	for _, s := range steps {
		var p string
		switch s.Kind {
		case migrate.CreateTable:
			p = "create_" + snake(tableWord(s.Table))
		case migrate.DropTable:
			p = "drop_" + snake(tableWord(s.Table))
		case migrate.RenameTable:
			p = "rename_" + snake(tableWord(s.Table))
		case migrate.AddColumn:
			p = "add_" + snake(s.Column)
		case migrate.DropColumn:
			p = "drop_" + snake(s.Column)
		case migrate.RenameColumn:
			p = "rename_" + snake(s.Column)
		case migrate.Rebuild:
			p = "change_" + snake(tableWord(s.Table))
		default:
			continue
		}
		if !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	slug := ""
	for _, p := range parts {
		next := p
		if slug != "" {
			next = slug + "_" + p
		}
		if len(next) > maxSlug {
			break
		}
		slug = next
	}
	if slug == "" && len(parts) > 0 { // one part longer than the limit: cut it
		slug = strings.TrimRight(parts[0][:maxSlug], "_")
	}
	if slug == "" {
		slug = "change"
	}
	return slug
}

// tableWord is the Alder name of SQL table name sql: "Post__Post" → "Post".
func tableWord(sql string) string {
	if i := strings.LastIndex(sql, "__"); i >= 0 {
		return sql[i+2:]
	}
	return sql
}

// snake is name in lower case with `_` before each inner capital and in place of anything
// that is not a letter or digit: "publishedAt" → "published_at", "URLs" → "urls".
func snake(name string) string {
	var b strings.Builder
	rs := []rune(name)
	for i, r := range rs {
		switch {
		case unicode.IsUpper(r):
			if i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		case r < 128 && (unicode.IsLower(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return strings.Trim(s, "_")
}
