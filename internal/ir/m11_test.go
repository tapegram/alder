package ir

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestIR_M11Unchanged (M15): `@owner` is `@partition user` and the `mine` family is the scoped
// operations with the key `user`, so the IR of the M11 apps changes only in how those are
// recorded. testdata/golden/m11 holds the dumps as M11 (bffe7d0) printed them; the dumps of today,
// read back through m11View, must equal them. The exact difference, and all of it:
//
//	(owner C)                          in a table form  ←  (partition user C)
//	(db-mine T)                                         ←  (db-scoped Db.mine user T)
//	(db-get-mine T …)                                   ←  (db-get-scoped Db.getMine user T …)
//	(db-update-mine T …)                                ←  (db-update-scoped Db.updateMine user T …)
//	(db-delete-mine T …)                                ←  (db-delete-scoped Db.deleteMine user T …)
//	(db-insert-mine T …)                                ←  (db-insert-scoped Db.insertMine user T …)
//
// plus the line breaks the longer forms move (both sides are compared with every run of
// white space read as one space). A program without `silo` prints no store for its arms, and
// `(requires user)` is printed as before.
func TestIR_M11Unchanged(t *testing.T) {
	for _, c := range []struct{ name, dir string }{
		{"todos", ""}, // TestIR_Todos's copy, without the generated admin
		{"blog", "../../examples/blog"},
		{"rules-ok", "../../testdata/corpus/rules-ok"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := c.dir
			if dir == "" {
				dir = todosWithoutAdmin(t)
			}
			want, err := os.ReadFile("../../testdata/golden/m11/" + c.name + ".ir")
			if err != nil {
				t.Fatal(err)
			}
			got := m11View(Dump(lowerDir(t, dir)))
			if w := spaces(string(want)); got != w {
				i := 0
				for i < len(got) && i < len(w) && got[i] == w[i] {
					i++
				}
				t.Fatalf("the %s dump differs from M11's beyond the partition records, at byte %d:\n got …%s…\nwant …%s…", c.name, i, excerpt(got, i), excerpt(w, i))
			}
		})
	}
}

var (
	wsRun     = regexp.MustCompile(`\s+`)
	partition = regexp.MustCompile(`\(partition user ([a-zA-Z0-9_]+)\)`)
	scopedOld = strings.NewReplacer(
		"(db-scoped Db.mine user ", "(db-mine ",
		"(db-get-scoped Db.getMine user ", "(db-get-mine ",
		"(db-update-scoped Db.updateMine user ", "(db-update-mine ",
		"(db-delete-scoped Db.deleteMine user ", "(db-delete-mine ",
		"(db-insert-scoped Db.insertMine user ", "(db-insert-mine ",
	)
)

func spaces(s string) string { return wsRun.ReplaceAllString(s, " ") }

// m11View reads a dump of today as M11 printed it (see TestIR_M11Unchanged), white space runs as
// one space.
func m11View(s string) string {
	s = spaces(s)
	s = partition.ReplaceAllString(s, "(owner $1)")
	return scopedOld.Replace(s)
}

func excerpt(s string, i int) string {
	lo, hi := max(i-120, 0), min(i+120, len(s))
	return s[lo:hi]
}
