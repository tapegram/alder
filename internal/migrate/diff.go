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
	IntoSilo                     // a table moves into `silo NewName` (SiloMigration): CREATE TABLE IF NOT EXISTS
)

var stepKindNames = [...]string{"CreateTable", "DropTable", "RenameTable", "AddColumn", "DropColumn", "RenameColumn", "AddIndex", "DropIndex", "Rebuild", "IntoSilo"}

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
		if n := display(s.NewName); n != t {
			return "rename table " + t + " to " + n
		}
		// the same short name: the table moved to another module, so say where from and to
		return "rename table " + strings.ReplaceAll(s.Table, "__", ".") + " to " + strings.ReplaceAll(s.NewName, "__", ".")
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
	case IntoSilo:
		return "move table " + t + " into silo " + s.NewName
	}
	return s.Kind.String() + " " + t
}

// FormatChanged is the refusal of a snapshot of another storage format (E0019's text, and
// Diff's): moving every table to a new format is a rebuild of each, which `alder migrate`
// does not write yet.
func FormatChanged(old, new string) string {
	return fmt.Sprintf("the storage format changed (v%s → v%s): this needs a rebuild migration of every table, which this `alder migrate` cannot write yet (`alder migrate --format` is planned); build with the compiler that wrote migrations/ until then", old, new)
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
	from    map[string]string // new table name → old table name (same name, @renamed, or same bare name: moved)
	claimed map[string]bool   // old table names with a new table
	renamed map[string]string // old table name → new, for the renamed ones
}

// pairTables matches new's tables to old's: by name, else by RenamedFrom, else by bare name (a
// table moved to another module). A RenamedFrom naming no table of old (when new's own name is
// not in old either) is an error.
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
		from := old.renamedFrom(t.RenamedFrom)
		switch {
		case old.table(from) == nil:
			errs = append(errs, fmt.Errorf("table %s: `@renamed %q`, but the snapshot has no table %s", short(t.Name), short(t.RenamedFrom), short(t.RenamedFrom)))
		case p.claimed[from]:
			errs = append(errs, fmt.Errorf("table %s: `@renamed %q`, but table %s is still declared (or renamed to twice)", short(t.Name), short(t.RenamedFrom), short(t.RenamedFrom)))
		default:
			p.from[t.Name], p.claimed[from] = from, true
			p.renamed[from] = t.Name
		}
	}
	// a new table still unpaired whose bare name is that of exactly one unclaimed old table is
	// that table moved to another module (`Todo.Todo` → `Table.Todo.Todo`): table names are
	// unique app-wide, so the bare name identifies it. Two candidates: no pairing.
	for _, t := range new.Tables {
		if _, ok := p.from[t.Name]; ok {
			continue
		}
		var cand []string
		for _, o := range old.Tables {
			if !p.claimed[o.Name] && short(o.Name) == short(t.Name) {
				cand = append(cand, o.Name)
			}
		}
		if len(cand) == 1 {
			p.from[t.Name], p.claimed[cand[0]] = cand[0], true
			p.renamed[cand[0]] = t.Name
		}
	}
	// a table kept under its name whose @renamed names another table of the snapshot that
	// nothing claims: the rename cannot be made (both names exist), and ignoring it would keep
	// the wrong table's rows
	for _, t := range new.Tables {
		if p.from[t.Name] == t.Name && t.RenamedFrom != "" && t.RenamedFrom != t.Name && old.table(old.renamedFrom(t.RenamedFrom)) != nil && !p.claimed[old.renamedFrom(t.RenamedFrom)] {
			errs = append(errs, renameBothExist("table "+short(t.Name), short(t.Name), short(t.RenamedFrom)))
		}
	}
	return p, errors.Join(errs...)
}

// renameBothExist is the error for `n @renamed "o"` when the snapshot has both o and n.
func renameBothExist(what, n, o string) error {
	return fmt.Errorf("%s: `%s @renamed %q`: both `%s` and `%s` exist in the snapshot; a rename needs the old name to be gone — remove `@renamed` if `%s` is new, or drop `%s` first", what, n, o, o, n, n, n)
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
	for _, c := range n.Columns {
		if _, kept := oix[c.Name]; kept && c.RenamedFrom != "" && c.RenamedFrom != c.Name {
			if _, ok := oix[c.RenamedFrom]; ok && claimed[c.RenamedFrom] == "" {
				errs = append(errs, renameBothExist(tn+"."+c.Name, c.Name, c.RenamedFrom))
			}
		}
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
			case cp.n.Default != nil && *cp.n.Default != "NULL":
				// non-null, or a Maybe with a `Just` default: existing rows get the default,
				// which ADD COLUMN cannot write without leaving DEFAULT in the table's text
				tp.reasons = append(tp.reasons, fmt.Sprintf("%s: new column filled with %s", label, *cp.n.Default))
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
			case cp.o == nil && cp.n.Nullable && !cp.n.Unique && (cp.n.Default == nil || *cp.n.Default == "NULL"):
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
		return nil, errors.New(FormatChanged(old.Format, new.Format))
	}
	old, new = sorted(old), sorted(new)
	p, err := pairTables(old, new)
	if err != nil {
		return nil, err
	}
	if err := idChanges(old, new, p); err != nil {
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

// sorted is s with its tables sorted by name (Diff is a function of the set of tables).
func sorted(s Schema) Schema {
	s.Tables = slices.Clone(s.Tables)
	slices.SortFunc(s.Tables, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

// idChanges reports a referenced table whose @id is a different column than before (not the
// same one renamed): the rows referring to it hold the old ids, which no migration can map.
func idChanges(old, new Schema, p pairing) error {
	var errs []error
	for _, n := range new.Tables {
		from, ok := p.from[n.Name]
		if !ok {
			continue
		}
		o := old.table(from)
		idFrom := ""
		for _, c := range n.Columns {
			if c.Name == n.ID {
				if o.col(c.Name) != nil {
					idFrom = c.Name
				} else if c.RenamedFrom != "" && o.col(c.RenamedFrom) != nil {
					idFrom = c.RenamedFrom
				}
			}
		}
		if idFrom == o.ID {
			continue
		}
		var refs []string
		for _, t := range new.Tables {
			if slices.ContainsFunc(t.Columns, func(c Column) bool { return c.Ref == n.Name }) {
				refs = append(refs, short(t.Name))
			}
		}
		if len(refs) > 0 {
			errs = append(errs, fmt.Errorf("%s's id column changed: tables referencing it (%s) keep the old ids; this is not migrated", short(n.Name), strings.Join(refs, ", ")))
		}
	}
	return errors.Join(errs...)
}

// col is t's column named name, or nil.
func (t *Table) col(name string) *Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
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
