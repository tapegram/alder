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
// A table that migrations/ already create lives in the shared database until now, and a
// database may hold rows of it: it is refused unless empty (`--empty`: the person says no
// database holds rows of it yet). Then its schema.json entry is marked with the silo (the
// migration generator refuses a table that changes store without that mark, book 8 "Silos"), a
// migration is written (migrate.SiloMigration: it creates the table in the silos made before,
// which are behind until it runs, and DROPS DATA, so a deploy of it needs --allow-data-loss),
// and the notes say what stays behind: rows a shared database holds of it are not moved.
func NewSilo(p *Program, key string, tables []string, empty bool) ([]Edit, []string, error) {
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
			return nil, nil, partitionFix(p, t, kt, key)
		}
		seen[name] = true
	}

	e, err := siloApp(app, key, tables, pluralsOf(p, tables))
	if err != nil {
		return nil, nil, err
	}
	return siloEdits(p, app, e, key, tables, empty)
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
				if len(d.TableSpans) == 0 {
					return nil, Refusef("`App`'s `silo %s` has no table: add the tables to it yourself", key)
				}
				appendItem(e, d.Span, d.TableSpans, strings.Join(tables, ", "))
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
func partitionFix(p *Program, t, kt *Table, key string) error {
	for _, c := range t.Columns {
		if c.Ref == kt.Name && !c.Maybe {
			return Refusef("`%s` is not partitioned by `%s`: mark its column `%s` `@partition %s` (`%s: %s @ref %s @partition %s`), then run this again", t.Name, key, c.Name, key, c.Name, typeText(p, c.Type, t.Module), kt.Name, key)
		}
	}
	return Refusef("`%s` is not partitioned by `%s`: it has no `@ref %s` column, and a required reference cannot be added to a table that may have rows; one with none yet is made again with it: `alder remove table %s`, then `alder new table %s … %s:%s.%sId@ref=%s@partition=%s`", t.Name, key, kt.Name, t.Name, t.Name, key, kt.Name, kt.Name, kt.Name, key)
}

// siloEdits are App's edit and, when migrations/schema.json has the tables (migrations create
// them in the shared database), with empty, the snapshot's tables marked with the silo and the
// migration that creates them in the silos made before, with the notes that say what that
// leaves behind; without empty, the refusal.
func siloEdits(p *Program, app *Module, e *srcEdit, key string, tables []string, empty bool) ([]Edit, []string, error) {
	edits := []Edit{{Path: app.File, Body: e.result()}}
	dir := filepath.Join(p.Dir, "migrations")
	snap := filepath.Join(dir, "schema.json")
	if _, err := os.Stat(snap); err != nil {
		return edits, nil, nil
	}
	s, err := migrate.ReadSnapshot(snap)
	if err != nil {
		return nil, nil, err
	}
	var moved, qualified []string
	for i := range s.Tables {
		for _, n := range tables {
			if t := p.TableNamed(n); t != nil && s.Tables[i].Name == t.Module+"."+t.Name {
				s.Tables[i].Silo = key
				moved = append(moved, n)
				qualified = append(qualified, s.Tables[i].Name)
			}
		}
	}
	if len(moved) == 0 {
		return edits, nil, nil
	}
	slices.Sort(moved)
	slices.Sort(qualified)
	them := map[bool]string{true: "it", false: "them"}[len(moved) == 1]
	if !empty {
		return nil, nil, Refusef("%s: migrations/ create %s in the shared database, so a database may hold rows of %s: `silo %s` would leave them there, unread (a migration does not move rows between databases). If no database holds rows of %s yet, run this again with --empty: it also writes the migration that creates %s in the silos made before, and the deploy that applies it needs --allow-data-loss (book 8, \"Silos\")",
			joinAnd(moved), them, them, key, them, them)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	name, err := migrate.NextName(dir, "silo_"+key)
	if err != nil {
		return nil, nil, err
	}
	m, err := migrate.SiloMigration(s, key, qualified, name)
	if err != nil {
		return nil, nil, err
	}
	edits = append(edits, Edit{Path: "migrations/schema.json", Body: string(b) + "\n"},
		Edit{Path: "migrations/" + name + ".sql", Create: true, Body: migrate.Render(m)})
	notes := []string{fmt.Sprintf("!! %s: migrations/ create %s in the shared database; %s creates %s in the silos made before, and DROPS DATA: rows a database holds of %s stay in the shared database, unread, so its deploy needs --allow-data-loss (book 8, \"Silos\")",
		joinAnd(moved), them, name, them, them)}
	return edits, notes, nil
}
