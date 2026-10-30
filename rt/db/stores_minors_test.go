package db

import (
	"slices"
	"testing"
)

// #35: a DROPS DATA line gates a silo only when it names one of the silo's tables whole: the
// table "Project" is not "ProjectX", nor is "Project." the end of "SubProject.".
func TestMentionsNamesTablesWhole(t *testing.T) {
	names := map[string]bool{"Table__Project": true}
	for _, c := range []struct {
		line string
		want bool
	}{
		// a real drop stays gated
		{"table Project", true},
		{"column Project.name", true},
		{"Project.points: Int -> Float", true},
		{"column Other.x; column Project.y", true},
		{"column Project.name; column Other.x", true},
		// other tables whose names contain it do not
		{"table ProjectX", false},
		{"column ProjectX.name", false},
		{"column SubProject.name", false},
		{"table SubProject", false},
		{"ProjectX.points: Int -> Float", false},
		{"shared rows of Project: it moves into silo org, and the rows the shared database holds of it stay there, unread", false},
	} {
		if got := mentions(c.line, names); got != c.want {
			t.Errorf("mentions(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// #35: a dropped index on a column whose name has an underscore ("Project__org_id") is still the
// silo table's: the statement is kept for a silo table and left out for another's.
func TestStatementsDropIndexUnderscoreColumn(t *testing.T) {
	ss := &siloSchema{}
	m := Migration{Name: "0002", SQL: `DROP INDEX IF EXISTS "Table__Project__org_id";` + "\n" + `DROP INDEX IF EXISTS "Table__Other__org_id";`}
	got := ss.statements(m, map[string]bool{"Table__Project": true})
	want := []string{`DROP INDEX IF EXISTS "Table__Project__org_id"`}
	if !slices.Equal(got, want) {
		t.Fatalf("statements = %q, want %q", got, want)
	}
}

// #35: a chain of renames within one migration (A to B, then B to C) names A as the silo's
// table too, when the build's silo has C.
func TestPendingNamesRenameChain(t *testing.T) {
	ss := &siloSchema{sqlName: map[string]bool{"C": true}}
	ms := []Migration{{Name: "0002", SQL: `ALTER TABLE "A" RENAME TO "B";` + "\n" + `ALTER TABLE "B" RENAME TO "C";`}}
	names := ss.pendingNames(ms, nil)
	for _, n := range []string{"A", "B", "C"} {
		if !names[0][n] {
			t.Errorf("names = %v, missing %s", names[0], n)
		}
	}
	// and the walk back continues from A: an earlier migration sees the table as A
	ms = append([]Migration{{Name: "0001", SQL: `ALTER TABLE "A" ADD COLUMN "x" TEXT;`}}, ms...)
	names = ss.pendingNames(ms, nil)
	if !names[0]["A"] || names[0]["C"] || names[0]["B"] {
		t.Errorf("before the chain: names = %v, want only A", names[0])
	}
}
