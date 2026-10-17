package migrate

import (
	"fmt"
	"strings"

	"alder/rt/db"
)

// quote makes a double-quoted SQL identifier (as rt/db does).
func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

// literal makes a single-quoted SQL string literal (as rt/db does for enum constructors).
func literal(s string) string { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }

func createIndex(sqlTable, col string) string {
	return "CREATE INDEX IF NOT EXISTS " + quote(IndexName(sqlTable, col)) + " ON " + quote(sqlTable) + "(" + quote(col) + ")"
}

// SQL renders one step for SQLite; a Rebuild is the documented procedure (create T__alder__new,
// INSERT … SELECT with conversions, DROP, RENAME, recreate indexes). Every statement is one line
// without the trailing `;`. The statements expect foreign keys to be off while they run (the
// runtime turns them off and runs PRAGMA foreign_key_check afterwards).
func SQL(step Step, old, new Schema) ([]string, error) {
	t := quote(step.Table)
	switch step.Kind {
	case CreateTable:
		nt, err := newTable(step, new)
		if err != nil {
			return nil, err
		}
		st, err := db.CreateStmt(nt.Desc(), new.byName())
		return []string{st}, err
	case DropTable:
		return []string{"DROP TABLE " + t}, nil
	case RenameTable:
		return []string{"ALTER TABLE " + t + " RENAME TO " + quote(step.NewName)}, nil
	case AddColumn:
		nt, err := newTable(step, new)
		if err != nil {
			return nil, err
		}
		def, err := columnDef(*nt, step.Column, new)
		if err != nil {
			return nil, err
		}
		return []string{"ALTER TABLE " + t + " ADD COLUMN " + def}, nil
	case DropColumn:
		return []string{"ALTER TABLE " + t + " DROP COLUMN " + quote(step.Column)}, nil
	case RenameColumn:
		return []string{"ALTER TABLE " + t + " RENAME COLUMN " + quote(step.Column) + " TO " + quote(step.NewName)}, nil
	case AddIndex:
		return []string{createIndex(step.Table, step.Column)}, nil
	case DropIndex:
		return []string{"DROP INDEX IF EXISTS " + quote(IndexName(step.Table, step.Column))}, nil
	case Rebuild:
		return rebuild(step, old, new)
	case IntoSilo:
		nt, err := newTable(step, new)
		if err != nil {
			return nil, err
		}
		st, err := db.CreateStmt(nt.Desc(), new.byName())
		if err != nil {
			return nil, err
		}
		out := []string{strings.Replace(st, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1)}
		for _, c := range nt.Columns {
			if c.Ref != "" {
				out = append(out, createIndex(step.Table, c.Name))
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("migrate: unknown step kind %v", step.Kind)
}

// newTable is the table of new whose SQL name is step.Table.
func newTable(step Step, new Schema) (*Table, error) {
	for i := range new.Tables {
		if db.SQLName(new.Tables[i].Name) == step.Table {
			return &new.Tables[i], nil
		}
	}
	return nil, fmt.Errorf("migrate: %v step: no table %s in the new schema", step.Kind, step.Table)
}

// columnDef is column col's definition exactly as db.CreateStmt writes it inside t's CREATE
// TABLE, so ALTER TABLE ADD COLUMN leaves the statement SQLite stores equal to CreateStmt's.
// It is cut out of the statement of a table of t's @id column and col alone (an @id is a
// `brand Text`: its definition holds no ", ").
func columnDef(t Table, col string, s Schema) (string, error) {
	var id, c *Column
	for i := range t.Columns {
		switch t.Columns[i].Name {
		case t.ID:
			id = &t.Columns[i]
		case col:
			c = &t.Columns[i]
		}
	}
	if id == nil || c == nil {
		return "", fmt.Errorf("migrate: table %s has no column %s (or no @id)", short(t.Name), col)
	}
	by := s.byName()
	only := Table{Name: t.Name, ID: t.ID, Columns: []Column{*id}}
	pair := Table{Name: t.Name, ID: t.ID, Columns: []Column{*id, *c}}
	a, err := db.CreateStmt(only.Desc(), by)
	if err != nil {
		return "", err
	}
	b, err := db.CreateStmt(pair.Desc(), by)
	if err != nil {
		return "", err
	}
	prefix, suffix := strings.TrimSuffix(a, ") STRICT")+", ", ") STRICT"
	if !strings.HasPrefix(b, prefix) || !strings.HasSuffix(b, suffix) {
		return "", fmt.Errorf("migrate: cannot cut column %s out of %q", col, b)
	}
	return strings.TrimSuffix(strings.TrimPrefix(b, prefix), suffix), nil
}

// rebuild is SQLite's procedure for a change ALTER TABLE cannot make
// (https://www.sqlite.org/lang_altertable.html#otheralter):
//
//	CREATE TABLE "T__alder__new" (…new definition…) STRICT
//	INSERT INTO "T__alder__new" (cols) SELECT <converted old columns> FROM "T" [WHERE …]
//	DROP TABLE "T"
//	ALTER TABLE "T__alder__new" RENAME TO "T"
//	CREATE INDEX IF NOT EXISTS … (the table's @ref indexes, dropped with "T")
//
// A new column is filled with its default (NULL for a Maybe without one); a kept one with its
// old value converted (convert). The WHERE drops the rows that cannot be kept.
func rebuild(step Step, old, new Schema) ([]string, error) {
	n, err := newTable(step, new)
	if err != nil {
		return nil, err
	}
	old, new = sorted(old), sorted(new)
	p, err := pairTables(old, new)
	if err != nil {
		return nil, err
	}
	from, ok := p.from[n.Name]
	if !ok {
		return nil, fmt.Errorf("migrate: rebuild of %s: the table is new", short(n.Name))
	}
	tp, err := planTable(old.table(from), n, p, old, new)
	if err != nil {
		return nil, err
	}
	tmp := *n
	tmp.Name = n.Name + "__alder__new" // SQL "T__alder__new": no @ref index "T__<col>" can be named so (a name has no "__")
	create, err := db.CreateStmt(tmp.Desc(), new.byName())
	if err != nil {
		return nil, err
	}
	sqlT, sqlTmp := quote(step.Table), quote(db.SQLName(tmp.Name))
	var cols, exprs, conds []string
	for _, cp := range tp.cols {
		cols = append(cols, quote(cp.n.Name))
		if cp.o == nil {
			if cp.n.Default != nil {
				exprs = append(exprs, *cp.n.Default)
			} else {
				exprs = append(exprs, "NULL")
			}
			continue
		}
		cv, err := convert(short(n.Name), *cp.o, *cp.n)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, cv.expr)
		if cv.cond != "" {
			conds = append(conds, cv.cond)
		}
	}
	ins := "INSERT INTO " + sqlTmp + " (" + strings.Join(cols, ", ") + ") SELECT " + strings.Join(exprs, ", ") + " FROM " + sqlT
	if len(conds) > 0 {
		ins += " WHERE " + strings.Join(conds, " AND ")
	}
	out := []string{create, ins, "DROP TABLE " + sqlT, "ALTER TABLE " + sqlTmp + " RENAME TO " + sqlT}
	for _, c := range n.Columns {
		if c.Ref != "" {
			out = append(out, createIndex(step.Table, c.Name))
		}
	}
	return out, nil
}

// conversion is how a kept column's old values become its new ones in a rebuild: expr is the
// SELECT expression, cond a WHERE condition keeping only the rows that can be kept ("" for
// all), drops what is lost ("" for nothing).
type conversion struct{ expr, cond, drops string }

// convert is the conversion of column o of table tn (its Alder name) to n.
func convert(tn string, o, n Column) (conversion, error) {
	src := quote(o.Name)
	label := tn + "." + n.Name
	cv := conversion{expr: src}
	inList := func(names []string) string {
		lits := make([]string, len(names))
		for i, e := range names {
			lits[i] = literal(e)
		}
		return src + " IN (" + strings.Join(lits, ", ") + ")"
	}
	switch from, to := o.Type, n.Type; {
	case from == to && from != db.CEnum:
	case from == db.CEnum && to == db.CEnum:
		var gone []string
		for _, e := range o.Enum {
			if !contains(n.Enum, e) {
				gone = append(gone, e)
			}
		}
		if len(gone) > 0 {
			cv.cond = inList(n.Enum)
			cv.drops = "rows whose " + label + " is " + strings.Join(gone, " or ") + " are deleted (to keep them, add an UPDATE first)"
		}
	case to == db.CText && (from == db.CEnum || from == db.CDate || from == db.CTime):
	case to == db.CText && (from == db.CInt || from == db.CFloat):
		cv.expr = "CAST(" + src + " AS TEXT)"
	case to == db.CEnum && from == db.CText:
		cv.cond = inList(n.Enum)
		cv.drops = "rows whose " + label + " is not " + strings.Join(n.Enum, " or ") + " are deleted (to keep them, add an UPDATE first)"
	case to == db.CInt && from == db.CBool:
	case to == db.CFloat && (from == db.CInt || from == db.CBool):
		cv.expr = "CAST(" + src + " AS REAL)"
	case to == db.CInt && from == db.CFloat:
		cv.expr = "CAST(" + src + " AS INTEGER)"
		cv.drops = label + ": fractions are cut off"
	case to == db.CInt && from == db.CText:
		cv.expr = "CAST(" + src + " AS INTEGER)"
		cv.drops = label + ": text is cut to its leading whole number ('7.5' -> 7, '12abc' -> 12), and text that is not a number becomes 0"
	case to == db.CFloat && from == db.CText:
		cv.expr = "CAST(" + src + " AS REAL)"
		cv.drops = label + ": text is cut to its leading number ('12abc' -> 12.0), and text that is not a number becomes 0"
	case to == db.CBool && (from == db.CInt || from == db.CFloat):
		cv.expr = "(" + src + " <> 0)"
		cv.drops = label + ": numbers other than 0 and 1 become True"
	default:
		return cv, fmt.Errorf("%s: no conversion from %s to %s: remove the field and add one under a new name", label, typeName(Column{Type: o.Type, Enum: o.Enum}), typeName(Column{Type: n.Type, Enum: n.Enum}))
	}
	if o.Nullable {
		if cv.cond != "" {
			cv.cond = "(" + src + " IS NULL OR " + cv.cond + ")"
		}
		if !n.Nullable {
			if n.Default != nil && *n.Default != "NULL" {
				cv.expr = "COALESCE(" + cv.expr + ", " + *n.Default + ")"
			} else {
				cv.cond = join(cv.cond, src+" IS NOT NULL")
				cv.drops = join2(cv.drops, "rows whose "+label+" is Nothing are deleted")
			}
		}
	}
	return cv, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// join joins two WHERE conditions with AND; either may be "".
func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " AND " + b
}

// join2 joins two DropsData texts with "; "; either may be "".
func join2(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}
