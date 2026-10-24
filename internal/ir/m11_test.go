package ir

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestIR_M11Unchanged (M15): `@owner` is `@partition user` and the `mine` family is the scoped
// operations with the key `user`, so the IR of the M11 apps changes only in how those are
// recorded. (todos.ir and blog.ir were rewritten at #20, when the examples' tables moved to
// app/Table/ and their names gained the `Table.` prefix, which also changes how types print.) testdata/golden/m11 holds the dumps as M11 (bffe7d0) printed them; the dumps of today,
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

// TestIR_Partitions (M15): the corpus app partitions-ok lowers its partitions (org and user
// keys), `via`, `silo`, `choose`, the admin form and the stores of its update arms. Its dump is
// reviewed by hand in testdata/golden/ir_partitions.ir.
func TestIR_Partitions(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/partitions-ok")
	compareGolden(t, "../../testdata/golden/ir_partitions.ir", Dump(p))
	parts := map[string]string{}
	for _, tb := range p.Tables {
		if tb.Partition != nil {
			parts[tb.Name] = tb.Partition.Key + ":" + tb.Partition.Column
		}
	}
	if fmt.Sprint(parts) != "map[Issue:org:org Label:org:org Note:user:owner Project:org:org]" {
		t.Errorf("partitions: %v", parts)
	}
	if p.Via["org"] == nil || p.Via["org"].Name != "Membership" || len(p.Via) != 1 {
		t.Errorf("via: %v", p.Via)
	}
	var silo []string
	for _, tb := range p.Silos["org"] {
		silo = append(silo, tb.Name)
	}
	if strings.Join(silo, " ") != "Project Issue" || len(p.Silos) != 1 {
		t.Errorf("silos: %v", p.Silos)
	}
	if p.Choose["org"] == nil || p.Choose["org"].Module != "Orgs.Index" {
		t.Errorf("choose: %v", p.Choose)
	}
	stores := map[string]string{}
	for _, pg := range p.Pages {
		if pg.Module == "Admin.Projects" && (pg.Silo != "org" || strings.Join(pg.Requires, ",") != "user") {
			t.Errorf("admin form: requires %v silo %q", pg.Requires, pg.Silo)
		}
		for _, a := range pg.Arms {
			stores[pg.Module+"."+a.Msg] = a.Store.String()
		}
	}
	want := map[string]string{"Projects.Index.Add": "silo org", "Projects.Index.Seed": "silo org", "Projects.Index.Both": "silo org",
		"Projects.Index.Tag": "shared", "Notes.Index.Add": "shared", "Orgs.Index.Pick": "shared", "Admin.Projects.Remove": "silo org"}
	for k, w := range want {
		if stores[k] != w {
			t.Errorf("arm %s: store %q, want %q", k, stores[k], w)
		}
	}
	if len(p.AppArms) != 1 || p.AppArms[0].Msg != "Leave" || p.AppArms[0].Store != StoreShared {
		t.Errorf("App arms: %+v", p.AppArms)
	}
	seen := map[string]int{}
	for _, m := range p.Modules {
		for _, v := range m.Values {
			var walk func(e Expr)
			walk = func(e Expr) {
				switch x := e.(type) {
				case *DbScoped:
					seen[x.Op+" "+x.Key]++
				case *DbGetScoped:
					seen[x.Op+" "+x.Key]++
				case *DbUpdateScoped:
					seen[x.Op+" "+x.Key]++
				case *DbDeleteScoped:
					seen[x.Op+" "+x.Key]++
				case *DbInsertScoped:
					seen[x.Op+" "+x.Key]++
				}
				for _, k := range Children(e) {
					if k != nil {
						walk(k)
					}
				}
			}
			walk(v.Body)
		}
	}
	if fmt.Sprint(seen) != "map[Db.deleteScoped org:1 Db.getScoped org:2 Db.insertMine user:1 Db.insertScoped org:4 Db.insertScoped user:1 Db.mine user:1 Db.scoped org:3 Db.scoped user:1 Db.updateScoped org:2]" {
		t.Errorf("scoped nodes: %v", seen)
	}
}
