package migrate

import (
	"strings"
	"testing"
)

// named is a table with the given module-qualified name and a title column.
func named(name string, cols ...Column) Table {
	return Table{Name: name, ID: "id", Columns: append([]Column{text("id")}, cols...)}
}

func refTo(name, key string) Column { c := text(name); c.Ref = key; return c }

func moveSchemas() (old, new Schema) {
	old = schema(
		named("Todo.Todo", text("title"), refTo("owner", "User.User")),
		named("User.User", text("name")),
	)
	new = schema(
		named("Table.Todo.Todo", text("title"), refTo("owner", "Table.User.User")),
		named("Table.User.User", text("name")),
	)
	return old, new
}

func TestDiffMoveTablePairsByName(t *testing.T) {
	old, new := moveSchemas()
	m, err := Plan(old, new, "0002_move")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(m.SQL, "\n")
	for _, want := range []string{
		`ALTER TABLE "Todo__Todo" RENAME TO "Table__Todo__Todo"`,
		`ALTER TABLE "User__User" RENAME TO "Table__User__User"`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %s in:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "DROP TABLE") || strings.Contains(sql, "CREATE TABLE") {
		t.Errorf("a move must not drop or create tables:\n%s", sql)
	}
}

// TestDiffMoveTableKeepsRows applies the move to a seeded database (apply checks that every
// table keeps its rows and the result is exactly the new schema).
func TestDiffMoveTableKeepsRows(t *testing.T) {
	old, new := moveSchemas()
	m, err := Plan(old, new, "0002_move")
	if err != nil {
		t.Fatal(err)
	}
	got := apply(t, old, new, m)
	if !strings.Contains(got, "Todo=3") || !strings.Contains(got, "User=3") || !strings.Contains(got, "foreign_key_check: 0") {
		t.Errorf("rows not kept: %s", got)
	}
}

// TestDiffMoveAmbiguousNoPair: two old tables called Item leave a new Item unpaired.
func TestDiffMoveAmbiguousNoPair(t *testing.T) {
	old := schema(named("A.Item", text("x")), named("B.Item", text("x")))
	new := schema(named("Table.Item", text("x")))
	m, err := Plan(old, new, "0002_move")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.Join(m.SQL, "\n")
	if strings.Contains(sql, "RENAME TO") || !strings.Contains(sql, "DROP TABLE") || !strings.Contains(sql, "CREATE TABLE") {
		t.Errorf("ambiguous move must drop and create, got:\n%s", sql)
	}
}
