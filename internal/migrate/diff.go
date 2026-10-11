package migrate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"alder/rt/db"
)

// StepKind is the kind of a migration Step.
type StepKind int

const (
	CreateTable  StepKind = iota // CREATE TABLE (new's statement)
	DropTable                    // DROP TABLE; DropsData names the table
	RenameTable                  // ALTER TABLE … RENAME TO NewName
	AddColumn                    // ALTER TABLE … ADD COLUMN: a nullable, non-@unique column added after the others
	DropColumn                   // ALTER TABLE … DROP COLUMN: a column that is not @id, @unique or @ref
	RenameColumn                 // ALTER TABLE … RENAME COLUMN Column TO NewName
	AddIndex                     // CREATE INDEX IF NOT EXISTS "T__col" for a @ref column
	DropIndex                    // DROP INDEX IF EXISTS "T__col"
	Rebuild                      // SQLite's table rebuild: everything else (see SQL)
)

var stepKindNames = [...]string{"CreateTable", "DropTable", "RenameTable", "AddColumn", "DropColumn", "RenameColumn", "AddIndex", "DropIndex", "Rebuild"}

func (k StepKind) String() string {
	if int(k) < len(stepKindNames) {
		return stepKindNames[k]
	}
	return fmt.Sprintf("StepKind(%d)", int(k))
}

// Step is one change of a migration. Table is an SQL table name: the old one for DropTable,
// RenameTable and DropIndex, the new one otherwise (renames come first, so every later step
// sees the new name).
type Step struct {
	Kind      StepKind
	Table     string // SQL table name
	Column    string // for column and index steps (the old name for RenameColumn and DropIndex)
	NewName   string // renames
	Reason    string // Rebuild: "Post.body: Text -> Int", "Comment.post: delete rule @restrict -> @cascade"
	DropsData string // "" or what is lost: "column Post.summary", "table Draft", "Post.count: text that is not a number becomes 0"
}

// String is the step as one human line, for the migration file's header and `alder migrate`.
func (s Step) String() string {
	t := display(s.Table)
	switch s.Kind {
	case CreateTable:
		return "create table " + t
	case DropTable:
		return "drop table " + t
	case RenameTable:
		return "rename table " + t + " to " + display(s.NewName)
	case AddColumn:
		return "add column " + t + "." + s.Column
	case DropColumn:
		return "drop column " + t + "." + s.Column
	case RenameColumn:
		return "rename column " + t + "." + s.Column + " to " + s.NewName
	case AddIndex:
		return "add index on " + t + "." + s.Column
	case DropIndex:
		return "drop index on " + t + "." + s.Column
	case Rebuild:
		return "rebuild table " + t + ": " + s.Reason
	}
	return s.Kind.String() + " " + t
}

// display is the Alder name of SQL table name sql: "Post__Post" → "Post" (a table lives in the
// module named after it, and module names have no "__").
func display(sql string) string {
	if i := strings.LastIndex(sql, "__"); i >= 0 {
		return sql[i+2:]
	}
	return sql
}

// short is the Alder name of module-qualified table name: "Post.Post" → "Post".
func short(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}

// pairing is how new's tables descend from old's.
type pairing struct {
	from    map[string]string // new table name → old table name (same name, or @renamed)
	claimed map[string]bool   // old table names with a new table
	renamed map[string]string // old table name → new, for the renamed ones
}

// pairTables matches new's tables to old's: by name, else by RenamedFrom. A RenamedFrom naming
// no table of old (when new's own name is not in old either) is an error.
func pairTables(old, new Schema) (pairing, error) {
	p := pairing{from: map[string]string{}, claimed: map[string]bool{}, renamed: map[string]string{}}
	for _, t := range new.Tables {
		if old.table(t.Name) != nil {
			p.from[t.Name], p.claimed[t.Name] = t.Name, true
		}
	}
	var errs []error
	for _, t := range new.Tables {
		if _, ok := p.from[t.Name]; ok || t.RenamedFrom == "" {
			continue
		}
		switch {
		case old.table(t.RenamedFrom) == nil:
			errs = append(errs, fmt.Errorf("table %s: `@renamed %q`, but the snapshot has no table %s", short(t.Name), short(t.RenamedFrom), short(t.RenamedFrom)))
		case p.claimed[t.RenamedFrom]:
			errs = append(errs, fmt.Errorf("table %s: `@renamed %q`, but table %s is still declared (or renamed to twice)", short(t.Name), short(t.RenamedFrom), short(t.RenamedFrom)))
		default:
			p.from[t.Name], p.claimed[t.RenamedFrom] = t.RenamedFrom, true
			p.renamed[t.RenamedFrom] = t.Name
		}
	}
	return p, errors.Join(errs...)
}

// colPair is a column of the new table and the old column it descends from (nil: added).
type colPair struct {
	n   *Column
	o   *Column
	oix int // o's index in the old table, -1 when added
}

// tablePlan is how one matched table changes.
type tablePlan struct {
	o, n    *Table
	cols    []colPair // in new's order
	dropped []*Column // old columns with no new one, in old's order
	reasons []string  // why a rebuild is needed; none: ALTER TABLE steps suffice
	drops   []string  // what a rebuild loses
}

// planTable pairs the columns of matched tables o and n and decides whether the change needs a
// rebuild. tables is the table pairing (a @ref to a renamed table is not a change: SQLite's
// RENAME TO rewrites the REFERENCES clauses).
func planTable(o, n *Table, p pairing, old, new Schema) (*tablePlan, error) {
	tp := &tablePlan{o: o, n: n}
	tn := short(n.Name)
	oix := map[string]int{}
	for i, c := range o.Columns {
		oix[c.Name] = i
	}
	claimed := map[string]string{} // old column → "name" (kept under its name) or "rename"
	for _, c := range n.Columns {
		if _, ok := oix[c.Name]; ok {
			claimed[c.Name] = "name"
		}
	}
	var errs []error
	for i := range n.Columns {
		c := &n.Columns[i]
		cp := colPair{n: c, oix: -1}
		if j, ok := oix[c.Name]; ok {
			cp.o, cp.oix = &o.Columns[j], j
		} else if c.RenamedFrom != "" {
			j, ok := oix[c.RenamedFrom]
			switch {
			case !ok:
				errs = append(errs, fmt.Errorf("%s.%s: `@renamed %q`, but the snapshot's table %s has no column %s", tn, c.Name, c.RenamedFrom, tn, c.RenamedFrom))
			case claimed[c.RenamedFrom] == "name":
				errs = append(errs, fmt.Errorf("%s.%s: `@renamed %q`, but column %s is still declared", tn, c.Name, c.RenamedFrom, c.RenamedFrom))
			case claimed[c.RenamedFrom] == "rename":
				errs = append(errs, fmt.Errorf("%s.%s: `@renamed %q`, but another column is renamed from %s too", tn, c.Name, c.RenamedFrom, c.RenamedFrom))
			default:
				cp.o, cp.oix = &o.Columns[j], j
				claimed[c.RenamedFrom] = "rename"
			}
		}
		tp.cols = append(tp.cols, cp)
	}
	for i := range o.Columns {
		if claimed[o.Columns[i].Name] == "" {
			tp.dropped = append(tp.dropped, &o.Columns[i])
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// the @id
	idFrom := ""
	for _, cp := range tp.cols {
		if cp.n.Name == n.ID && cp.o != nil {
			idFrom = cp.o.Name
		}
	}
	if idFrom != o.ID {
		tp.reasons = append(tp.reasons, fmt.Sprintf("%s: @id %s -> %s", tn, o.ID, n.ID))
	}
	// changed columns
	lastMatched, firstAdded := -1, len(tp.cols)
	prevOld := -1
	reordered := false
	for i, cp := range tp.cols {
		label := tn + "." + cp.n.Name
		if cp.o == nil {
			firstAdded = min(firstAdded, i)
			switch {
			case cp.n.Ref != "" && !cp.n.Nullable:
				errs = append(errs, fmt.Errorf("%s: a new non-null `@ref` column needs a default so existing rows can be filled, and a reference has none: add it as `Maybe` first", label))
			case !cp.n.Nullable && cp.n.Default == nil:
				errs = append(errs, fmt.Errorf("%s: %s", label, needsDefault(*cp.n)))
			case !cp.n.Nullable:
				tp.reasons = append(tp.reasons, fmt.Sprintf("%s: new non-null column (filled with %s)", label, *cp.n.Default))
			case cp.n.Unique:
				tp.reasons = append(tp.reasons, label+": new @unique column")
			}
			continue
		}
		lastMatched = i
		if cp.oix < prevOld {
			reordered = true
		}
		prevOld = cp.oix
		tp.reasons = append(tp.reasons, changes(label, *cp.o, *cp.n, p, old, new)...)
	}
	if reordered || firstAdded < lastMatched {
		tp.reasons = append(tp.reasons, tn+": column order changed (ALTER TABLE only appends)")
	}
	for _, c := range tp.dropped {
		tp.drops = append(tp.drops, "column "+tn+"."+c.Name)
		switch {
		case c.Unique:
			tp.reasons = append(tp.reasons, tn+"."+c.Name+": dropped @unique column")
		case c.Ref != "":
			tp.reasons = append(tp.reasons, tn+"."+c.Name+": dropped @ref column")
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if len(tp.reasons) > 0 {
		// the rebuild also makes the table's simple changes: name them too
		for _, cp := range tp.cols {
			switch {
			case cp.o == nil && cp.n.Nullable && !cp.n.Unique:
				tp.reasons = append(tp.reasons, tn+"."+cp.n.Name+" added")
			case cp.o != nil && cp.o.Name != cp.n.Name:
				tp.reasons = append(tp.reasons, tn+"."+cp.o.Name+" renamed to "+cp.n.Name)
			}
		}
		for _, c := range tp.dropped {
			if !c.Unique && c.Ref == "" {
				tp.reasons = append(tp.reasons, tn+"."+c.Name+" dropped")
			}
		}
		for _, cp := range tp.cols {
			if cp.o == nil {
				continue
			}
			cv, err := convert(tn, *cp.o, *cp.n)
			if err != nil {
				errs = append(errs, err)
			} else if cv.drops != "" {
				tp.drops = append(tp.drops, cv.drops)
			}
		}
	}
	return tp, errors.Join(errs...)
}

// needsDefault is the error for a new non-null column without a default.
func needsDefault(c Column) string {
	ex := map[db.ColType]string{db.CInt: "0", db.CFloat: "0.0", db.CText: `""`, db.CBool: "False"}[c.Type]
	if c.Type == db.CEnum && len(c.Enum) > 0 {
		ex = c.Enum[0]
	}
	if ex == "" {
		return fmt.Sprintf("a new non-null column needs a default so existing rows can be filled, and a %s has no literal: add it as `Maybe` first", typeName(c))
	}
	return fmt.Sprintf("a new non-null column needs a default (`= %s`) so existing rows can be filled", ex)
}

// typeName is c's type for messages: "Int", "Maybe Text", "Draft | Published".
func typeName(c Column) string {
	s := colTypeNames[c.Type]
	if c.Type == db.CEnum {
		s = strings.Join(c.Enum, " | ")
		if c.Nullable {
			s = "(" + s + ")"
		}
	}
	if c.Nullable {
		s = "Maybe " + s
	}
	return s
}

// changes are the reasons matched columns o and n differ in what the CREATE TABLE statement
// says (a changed default is not one: defaults are not stored in the table).
func changes(label string, o, n Column, p pairing, old, new Schema) []string {
	var out []string
	if o.Type != n.Type || o.Nullable != n.Nullable || !slices.Equal(o.Enum, n.Enum) {
		out = append(out, fmt.Sprintf("%s: %s -> %s", label, typeName(o), typeName(n)))
	}
	if o.Unique != n.Unique {
		out = append(out, label+": @unique "+map[bool]string{true: "added", false: "removed"}[n.Unique])
	}
	oref := o.Ref
	if r, ok := p.renamed[oref]; ok {
		oref = r
	}
	switch {
	case oref == "" && n.Ref != "":
		out = append(out, label+": @ref "+short(n.Ref)+" added")
	case oref != "" && n.Ref == "":
		out = append(out, label+": @ref "+short(oref)+" removed")
	case oref != n.Ref:
		out = append(out, label+": @ref "+short(oref)+" -> @ref "+short(n.Ref))
	case n.Ref != "":
		if o.OnDelete != n.OnDelete {
			out = append(out, fmt.Sprintf("%s: delete rule %s -> %s", label, ruleName(o.OnDelete), ruleName(n.OnDelete)))
		}
		// REFERENCES names the referenced table's @id column: when that name changes, the
		// clause is rewritten by rebuilding this table too (a rebuild of the referenced table
		// does not touch it)
		if ot, nt := old.table(o.Ref), new.table(n.Ref); ot != nil && nt != nil && ot.ID != nt.ID {
			out = append(out, fmt.Sprintf("%s: the @id of %s is now %s", label, short(n.Ref), nt.ID))
		}
	}
	return out
}

func ruleName(r db.OnDelete) string {
	switch r {
	case db.Cascade:
		return "@cascade"
	case db.SetNull:
		return "@setNull"
	}
	return "@restrict"
}

// Diff plans the steps from old to new. Renames come from RenamedFrom; a column with
// RenamedFrom that does not exist in old is an error. Deterministic order: drops of tables,
// renames, creates, per-table column steps in declaration order, rebuilds, indexes.
//
// A table is rebuilt (one Rebuild step, which also carries its renames, drops and additions)
// when ALTER TABLE cannot give it new's exact CREATE TABLE text: a changed type, nullability,
// constructor list, @unique, @ref, delete rule or @id; a new column that is non-null (filled with
// its default) or @unique, or that is not after every kept column; a dropped @unique or @ref
// column; kept columns in a new order.
func Diff(old, new Schema) ([]Step, error) {
	if old.Format != "" && new.Format != "" && old.Format != new.Format {
		return nil, fmt.Errorf("the snapshot is storage format v%s, this compiler's is v%s", old.Format, new.Format)
	}
	p, err := pairTables(old, new)
	if err != nil {
		return nil, err
	}
	var steps []Step
	for _, t := range old.Tables {
		if !p.claimed[t.Name] {
			steps = append(steps, Step{Kind: DropTable, Table: db.SQLName(t.Name), DropsData: "table " + short(t.Name)})
		}
	}
	for _, t := range new.Tables {
		if from, ok := p.from[t.Name]; ok && from != t.Name {
			steps = append(steps, Step{Kind: RenameTable, Table: db.SQLName(from), NewName: db.SQLName(t.Name)})
		}
	}
	for _, t := range new.Tables {
		if _, ok := p.from[t.Name]; !ok {
			steps = append(steps, Step{Kind: CreateTable, Table: db.SQLName(t.Name)})
		}
	}
	var rebuilds []Step
	rebuilt := map[string]bool{} // new table names
	var errs []error
	for i := range new.Tables {
		n := &new.Tables[i]
		from, ok := p.from[n.Name]
		if !ok {
			continue
		}
		tp, err := planTable(old.table(from), n, p, old, new)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		sql := db.SQLName(n.Name)
		if len(tp.reasons) > 0 {
			rebuilt[n.Name] = true
			rebuilds = append(rebuilds, Step{Kind: Rebuild, Table: sql, Reason: strings.Join(tp.reasons, "; "), DropsData: strings.Join(tp.drops, "; ")})
			continue
		}
		for _, c := range tp.dropped {
			steps = append(steps, Step{Kind: DropColumn, Table: sql, Column: c.Name, DropsData: "column " + short(n.Name) + "." + c.Name})
		}
		for _, cp := range tp.cols {
			if cp.o != nil && cp.o.Name != cp.n.Name {
				steps = append(steps, Step{Kind: RenameColumn, Table: sql, Column: cp.o.Name, NewName: cp.n.Name})
			}
		}
		for _, cp := range tp.cols {
			if cp.o == nil {
				steps = append(steps, Step{Kind: AddColumn, Table: sql, Column: cp.n.Name})
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	steps = append(steps, rebuilds...)

	// indexes of @ref columns: a rebuilt table's come with its rebuild, a dropped table's go with it
	oldIdx, newIdx := indexes(old), indexes(new)
	var drops, adds []Step
	for _, ix := range oldIdx {
		nt := p.renamed[ix.table]
		if nt == "" && p.claimed[ix.table] {
			nt = ix.table
		}
		if nt == "" || rebuilt[nt] || slices.ContainsFunc(newIdx, func(n index) bool { return n.name() == ix.name() }) {
			continue
		}
		drops = append(drops, Step{Kind: DropIndex, Table: db.SQLName(ix.table), Column: ix.col})
	}
	for _, ix := range newIdx {
		if rebuilt[ix.table] || slices.ContainsFunc(oldIdx, func(o index) bool { return o.name() == ix.name() }) {
			continue
		}
		adds = append(adds, Step{Kind: AddIndex, Table: db.SQLName(ix.table), Column: ix.col})
	}
	steps = append(steps, drops...)
	return append(steps, adds...), nil
}

// index is the index of a @ref column.
type index struct{ table, col string }

func (ix index) name() string { return IndexName(db.SQLName(ix.table), ix.col) }

// IndexName is the name of the index of @ref column col of SQL table sqlTable: "T__col".
func IndexName(sqlTable, col string) string { return sqlTable + "__" + col }

// indexes are s's @ref column indexes, sorted by name.
func indexes(s Schema) []index {
	var out []index
	for _, t := range s.Tables {
		for _, c := range t.Columns {
			if c.Ref != "" {
				out = append(out, index{t.Name, c.Name})
			}
		}
	}
	slices.SortFunc(out, func(a, b index) int { return strings.Compare(a.name(), b.name()) })
	return out
}

// Indexes are the CREATE INDEX statements of s's @ref columns, sorted by index name; they are
// not part of the hash. (Ensure creates the same ones for an app without migrations.)
func (s Schema) Indexes() []string {
	var out []string
	for _, ix := range indexes(s) {
		out = append(out, createIndex(db.SQLName(ix.table), ix.col))
	}
	return out
}
