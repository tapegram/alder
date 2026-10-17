package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/migrate"
	"alder/internal/syntax"
)

// NewSilo generates `alder new silo <key> <T> …` (M15, book chapters 8 and 15): App's `silo key
// { T, … }`, written after the session (or the tables added to the declaration there is). Every
// table must be partitioned by key (`@partition key` on its `@ref` column to the key's table):
// one that is not is refused, naming the column to mark or to add. A table already in a silo is
// refused (a table lives in one store).
//
// A table that migrations/ already create lives in the shared database until now: its
// schema.json entry is marked with the silo (the migration generator refuses a table that
// changes store without that mark, book 8 "Silos"), and the notes say what stays behind: rows a
// shared database holds of it are not moved, and silos made before have no such table. So a
// silo is declared before the app holds data.
func NewSilo(p *Program, key string, tables []string) ([]Edit, []string, error) {
	app := p.Modules["App"]
	if app == nil || p.Session == nil {
		return nil, nil, Refusef("there is no module `App` with a `session`: a silo is keyed by a session field")
	}
	if !IsLowerIdent(key) {
		return nil, nil, Refusef("`%s` is not a session field's name: a silo is keyed by one, such as `org`", key)
	}
	f := p.SessionField(key)
	if f == nil {
		return nil, nil, Refusef("`silo %s`: the session has no field `%s`; `alder edit session add-field %s:<T>.<T>Id via <Membership>` adds one (book 6, \"Partitions\")", key, key, key)
	}
	if key == "user" {
		return nil, nil, Refusef("`silo user`: a database per user is not a silo this release makes; partition the tables by `user` (`@owner`) instead")
	}
	kt := p.KeyTable(key)
	if kt == nil {
		return nil, nil, Refusef("`silo %s`: the session's `%s` is not `Maybe` of a table's id (such as `Maybe Org.OrgId`), so no table names its values", key, key)
	}
	if len(tables) == 0 {
		return nil, nil, Refusef("`alder new silo %s` needs the tables that live in it: `alder new silo %s Project Task`", key, key)
	}
	seen := map[string]bool{}
	for _, name := range tables {
		t := p.TableNamed(name)
		switch {
		case t == nil:
			return nil, nil, Refusef("`%s` is not a table of this app (its tables: %s)", name, tableNames(p))
		case seen[name]:
			return nil, nil, Refusef("`%s` is given twice", name)
		case t.Silo == key:
			return nil, nil, Refusef("`%s` is in `silo %s` already", name, key)
		case t.Silo != "":
			return nil, nil, Refusef("`%s` is in `silo %s`: a table lives in one store", name, t.Silo)
		case t.PartKey != key:
			return nil, nil, partitionFix(t, kt, key)
		}
		seen[name] = true
	}

	e, err := siloApp(app, key, tables, pluralsOf(p, tables))
	if err != nil {
		return nil, nil, err
	}
	return siloEdits(p, app, e, key, tables)
}

// siloApp is App's source with `silo key { … }` written after the session (or the tables added
// to the declaration there is); plurals are the tables' rows in words, for its comment.
func siloApp(app *Module, key string, tables, plurals []string) (*srcEdit, error) {
	e := newSrcEdit(app.Source)
	var after int // the line the new declaration follows: the session's, or a login/choose/silo
	for _, d := range app.Syntax.Decls {
		switch d := d.(type) {
		case *syntax.SiloDecl:
			if d.Key == key {
				l := e.line(d.Span.End.Line)
				end := strings.LastIndex(l, "}")
				if d.Span.Start.Line != d.Span.End.Line || end < 0 {
					return nil, Refusef("`App`'s `silo %s` is not on one line: add the tables to it yourself", key)
				}
				inner := strings.TrimSuffix(strings.TrimSpace(l[strings.Index(l, "{")+1:end]), ",")
				e.replaceLine(d.Span.Start.Line, l[:strings.Index(l, "{")]+"{ "+inner+", "+strings.Join(tables, ", ")+" }"+l[end+1:])
				return e, nil
			}
			after = max(after, d.Span.End.Line)
		case *syntax.SessionDecl, *syntax.LoginDecl, *syntax.ChooseDecl:
			after = max(after, d.GetSpan().End.Line)
		}
	}
	line := "silo " + key + " { " + strings.Join(tables, ", ") + " }"
	comment := "-- " + UpperFirst(joinAnd(plurals)) + " live in a database per " + key + " (book 8, \"Silos\")."
	e.insertBefore(after+1, "\n"+comment+"\n"+line)
	return e, nil
}

// pluralsOf are tables' rows in a sentence: "projects".
func pluralsOf(p *Program, tables []string) []string {
	var out []string
	for _, n := range tables {
		out = append(out, pluralOf(p.TableNamed(n)))
	}
	return out
}

// partitionFix is the refusal of table t in `silo key`, which is not partitioned by key: the
// column to mark `@partition key`, or the one to add.
func partitionFix(t, kt *Table, key string) error {
	for _, c := range t.Columns {
		if c.Ref == kt.Name && !c.Maybe {
			return Refusef("`%s` is not partitioned by `%s`: mark its column `%s` `@partition %s` (`%s: %s @ref %s @partition %s`), then run this again", t.Name, key, c.Name, key, c.Name, c.TypeIn(t.Module), kt.Name, key)
		}
	}
	return Refusef("`%s` is not partitioned by `%s`: it has no `@ref %s` column, and a required reference cannot be added to a table that may have rows; one with none yet is made again with it: `alder remove table %s`, then `alder new table %s … %s:%s.%sId@ref=%s@partition=%s`", t.Name, key, kt.Name, t.Name, t.Name, key, kt.Module, kt.Name, kt.Name, key)
}

// siloEdits are App's edit and, when migrations/schema.json has the tables (migrations create
// them in the shared database), the snapshot's tables marked with the silo, with the notes that
// say what that leaves behind.
func siloEdits(p *Program, app *Module, e *srcEdit, key string, tables []string) ([]Edit, []string, error) {
	edits := []Edit{{Path: app.File, Body: e.result()}}
	snap := filepath.Join(p.Dir, "migrations", "schema.json")
	if _, err := os.Stat(snap); err != nil {
		return edits, nil, nil
	}
	s, err := migrate.ReadSnapshot(snap)
	if err != nil {
		return nil, nil, err
	}
	var moved []string
	for i := range s.Tables {
		for _, n := range tables {
			if t := p.TableNamed(n); t != nil && s.Tables[i].Name == t.Module+"."+t.Name {
				s.Tables[i].Silo = key
				moved = append(moved, n)
			}
		}
	}
	if len(moved) == 0 {
		return edits, nil, nil
	}
	slices.Sort(moved)
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	edits = append(edits, Edit{Path: "migrations/schema.json", Body: string(b) + "\n"})
	notes := []string{fmt.Sprintf("!! %s: migrations/ create %s in the shared database; a database that already holds rows of %s keeps them there, unread, and silos made before have no such table: declare a silo before the app holds data (book 8, \"Silos\")",
		joinAnd(moved), map[bool]string{true: "it", false: "them"}[len(moved) == 1], map[bool]string{true: "it", false: "them"}[len(moved) == 1])}
	return edits, notes, nil
}
