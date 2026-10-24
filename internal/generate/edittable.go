package generate

import (
	"strings"

	"alder/internal/syntax"
)

// EditTable generates `alder edit table <T> add-field <field> | rename-field <old> <new> |
// drop-field <field>`: the table's declaration edited (a field added after the last one; a field
// renamed with `@renamed "old"`, as chapter 8 says, so the
// migration keeps its data; a field's line removed). The migration is the caller's
// (Options.Migrate); the code the change breaks is the copy's diagnostics, which `alder edit
// table` prints and still writes (Options.WriteDespiteErrors) unless --strict.
func EditTable(p *Program, table, op string, args []string) ([]Edit, error) {
	t := p.TableNamed(table)
	if t == nil {
		return nil, Refusef("`%s` is not a table of this app (its tables: %s)", table, tableNames(p))
	}
	m := p.Modules[t.Module]
	e := newSrcEdit(m.Source)
	switch op {
	case "add-field":
		if len(args) != 1 {
			return nil, Refusef("`add-field` takes one field: `alder edit table %s add-field name:Type[=default][@attr…]`", table)
		}
		f, err := ParseField(args[0], true)
		if err != nil {
			return nil, Refusef("%v", err)
		}
		if t.Column(f.Name) != nil {
			return nil, Refusef("`%s` already has a field `%s`", table, f.Name)
		}
		if f.Has("id") {
			return nil, Refusef("`%s` already has its id, `%s`: a table has one", table, t.IDField)
		}
		// at the end: a column added last is an `ALTER TABLE … ADD COLUMN`, where one between
		// others is a rebuild of the table (chapter 8, "What a change becomes")
		after := t.Columns[len(t.Columns)-1].Decl
		imports := newImports(t.Module, p.Std)
		typeImports(imports, f.Type, p)
		for _, a := range f.Attrs {
			if a.Name == "ref" {
				imports.add(a.Arg)
			}
		}
		addImports(e, m.Syntax, imports.list())
		if ownLine(e, after) == nil { // on a line of its own, as `alder fmt` writes 3 fields or more
			e.insertBefore(after.Span.End.Line+1, indentOf(e.line(after.Span.Start.Line))+f.Source()+",")
		} else { // `table T { id: TId @id, name: Text }` on one line
			e.insertAt(after.Span.End.Line, after.Span.End.Col, ", "+f.Source())
		}
	case "rename-field":
		if len(args) != 2 {
			return nil, Refusef("`rename-field` takes the old name and the new: `alder edit table %s rename-field old new`", table)
		}
		c, err := column(t, args[0])
		if err != nil {
			return nil, err
		}
		if !IsLowerIdent(args[1]) {
			return nil, Refusef("`%s` is not a field name: a field name is a lower-case identifier", args[1])
		}
		if t.Column(args[1]) != nil {
			return nil, Refusef("`%s` already has a field `%s`", table, args[1])
		}
		if c.Renamed != "" {
			return nil, Refusef("`%s.%s` already has `@renamed %s`: once its migration is made (`alder migrate`), remove the attribute, then rename again", table, c.Name, quote(c.Renamed))
		}
		// the attribute after the field, then its name: right to left on a line it shares
		sp := c.Decl.Span
		e.insertAt(sp.End.Line, sp.End.Col, " @renamed "+quote(c.Name))
		l, col := e.current(sp.Start.Line), sp.Start.Col-1
		if !strings.HasPrefix(l[col:], c.Name) {
			return nil, Refusef("internal error: `%s.%s` is not where the parser says", table, c.Name)
		}
		e.replaceLine(sp.Start.Line, l[:col]+args[1]+l[col+len(c.Name):])
	case "drop-field":
		if len(args) != 1 {
			return nil, Refusef("`drop-field` takes the field's name: `alder edit table %s drop-field name`", table)
		}
		c, err := column(t, args[0])
		if err != nil {
			return nil, err
		}
		if c.ID {
			return nil, Refusef("`%s` is the id of `%s`: a table keeps its id", c.Name, table)
		}
		if ownLine(e, c.Decl) == nil {
			for n := c.Decl.Span.Start.Line; n <= c.Decl.Span.End.Line; n++ {
				e.deleteLine(n)
			}
		} else if err := dropShared(e, t, c); err != nil {
			return nil, err
		}
	default:
		return nil, Refusef("`%s` is not an edit: `alder edit table %s` takes add-field, rename-field or drop-field", op, table)
	}
	// an error in the table itself (the new field's type or default) refuses the edit; breakage
	// in other code is the to-do list the caller writes despite
	guard := func(d syntax.Decl) bool { td, ok := d.(*syntax.TableDecl); return ok && td.Name == table }
	return []Edit{{Path: m.File, Body: e.result(), Guard: guard}}, nil
}

// dropShared removes field c of table t from a line it shares with others (`table T { id: TId
// @id, a: Int, b: Text }`): from it to the next field, or from the field before it when it is the
// last.
func dropShared(e *srcEdit, t *Table, c *Column) error {
	var prev, next *syntax.Field
	for i, col := range t.Columns {
		if col.Decl == c.Decl {
			if i > 0 {
				prev = t.Columns[i-1].Decl
			}
			if i+1 < len(t.Columns) {
				next = t.Columns[i+1].Decl
			}
		}
	}
	from, to := c.Decl.Span.Start, c.Decl.Span.End
	switch {
	case next != nil:
		to = next.Span.Start
	case prev != nil:
		from = prev.Span.End
	}
	if from.Line != to.Line {
		return Refusef("`%s.%s` shares its lines with other fields in a way `alder edit table` cannot follow: drop the field yourself", t.Name, c.Name)
	}
	l := e.current(from.Line)
	e.replaceLine(from.Line, l[:from.Col-1]+l[to.Col-1:])
	return nil
}

// column is t's column name, refused when it has none.
func column(t *Table, name string) (*Column, error) {
	if c := t.Column(name); c != nil {
		return c, nil
	}
	var names []string
	for _, c := range t.Columns {
		names = append(names, c.Name)
	}
	return nil, Refusef("`%s` has no field `%s` (its fields: %s)", t.Name, name, strings.Join(names, ", "))
}

// ownLine checks that field f is written on lines of its own: nothing before it on its first
// line, nothing but its comma (and a comment) after it on its last.
func ownLine(e *srcEdit, f *syntax.Field) error {
	first, last := e.line(f.Span.Start.Line), e.line(f.Span.End.Line)
	rest := strings.TrimSpace(last[min(len(last), f.Span.End.Col-1):])
	if strings.TrimSpace(first[:f.Span.Start.Col-1]) != "" || !(rest == "," || rest == "" || strings.HasPrefix(rest, ", --") || strings.HasPrefix(rest, ",--")) {
		return Refusef("not one field per line")
	}
	return nil
}
