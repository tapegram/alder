// internal/names/qualify_test.go
package names

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestCanonicalQualifiers(t *testing.T) {
	std := map[string]bool{"List": true, "Maybe": true, "App": true}
	cases := []struct {
		name  string
		self  string
		paths []string
		extra string // an extra taken name (a local declaration)
		want  map[string]string
		bad   []string
	}{
		{"last segment", "M", []string{"Table.Todo"}, "", map[string]string{"Table.Todo": "Todo"}, nil},
		{"whole path beside suffix", "M", []string{"Layout", "Admin.Layout"}, "", map[string]string{"Layout": "Layout", "Admin.Layout": "AdminLayout"}, nil},
		{"two segments", "M", []string{"Posts.Index", "Admin.Posts.Index"}, "", map[string]string{"Posts.Index": "PostsIndex", "Admin.Posts.Index": "AdminPostsIndex"}, nil},
		{"stdlib name", "M", []string{"Utils.List"}, "", map[string]string{"Utils.List": "UtilsList"}, nil},
		{"self segment", "Sites.MnSos", []string{"Data.MnSos"}, "", map[string]string{"Data.MnSos": "DataMnSos"}, nil},
		{"own segment, full length", "Admin.Layout", []string{"Layout"}, "", map[string]string{"Layout": "Layout"}, nil},
		{"own segment, beside a longer path", "Admin.Layout", []string{"Layout", "Shop.Layout"}, "", map[string]string{"Layout": "Layout", "Shop.Layout": "ShopLayout"}, nil},
		{"local declaration", "M", []string{"Table.Todo"}, "Todo", map[string]string{"Table.Todo": "TableTodo"}, nil},
		{"a join beside its own suffix", "M", []string{"A.X", "AX"}, "", map[string]string{"A.X": "X", "AX": "AX"}, nil},
		{"local type named like the last segment", "Todos", []string{"Table.Todo", "Layout", "Admin.Layout"}, "Todo", map[string]string{"Table.Todo": "TableTodo", "Layout": "Layout", "Admin.Layout": "AdminLayout"}, nil},
		{"identical joins", "M", []string{"A.X", "B.X", "AX"}, "", map[string]string{"A.X": "", "B.X": "BX", "AX": ""}, []string{"A.X", "AX"}},
		{"distinct ends", "M", []string{"Todos.Index", "Projects.Index", "Blog.Show"}, "", map[string]string{"Todos.Index": "TodosIndex", "Projects.Index": "ProjectsIndex", "Blog.Show": "Show"}, nil},
		{"three suffixes of one name", "M", []string{"X", "A.X", "B.A.X"}, "", map[string]string{"X": "X", "A.X": "AX", "B.A.X": "BAX"}, nil},
		{"taken at k and colliding at k+1", "M", []string{"A.B.X", "C.B.X"}, "X", map[string]string{"A.B.X": "ABX", "C.B.X": "CBX"}, nil},
		{"self blocks nothing at the whole join", "Y.AX", []string{"A.X", "B.X"}, "", map[string]string{"A.X": "AX", "B.X": "BX"}, nil},
		{"whole join taken", "M", []string{"List"}, "", map[string]string{"List": ""}, []string{"List"}},
	}
	for _, c := range cases {
		taken := func(n string) bool { return std[n] || n == c.extra }
		got, bad := CanonicalQualifiers(c.self, c.paths, taken)
		if !reflect.DeepEqual(got, c.want) || !reflect.DeepEqual(bad, c.bad) {
			t.Errorf("%s: got %v bad %v, want %v bad %v", c.name, got, bad, c.want, c.bad)
		}
		rng := rand.New(rand.NewSource(1))
		for i := 0; i < 10; i++ {
			sh := append([]string(nil), c.paths...)
			rng.Shuffle(len(sh), func(a, b int) { sh[a], sh[b] = sh[b], sh[a] })
			g2, b2 := CanonicalQualifiers(c.self, sh, taken)
			if !reflect.DeepEqual(g2, got) || !reflect.DeepEqual(b2, bad) {
				t.Errorf("%s: order-dependent result for %v: %v bad %v", c.name, sh, g2, b2)
			}
		}
	}
}

func TestNeedsAlias(t *testing.T) {
	if NeedsAlias("Table.Todo", "Todo") || !NeedsAlias("Table.Todo", "TableTodo") || NeedsAlias("Layout", "Layout") {
		t.Error("NeedsAlias wrong")
	}
}

func TestTaken(t *testing.T) {
	m := parse(t, "M.alder", `module M

type Status = Open | Done
table Todo { title: String }
capability Clock { now: () -> Int }
fake Clock {
	table Charge { cents: Int }
	now = 1
}
`)
	taken := Taken(m, func(n string) bool { return n == "List" })
	for _, n := range []string{"App", "List", "Status", "Todo", "Clock", "Charge"} {
		if !taken(n) {
			t.Errorf("%s should be taken", n)
		}
	}
	for _, n := range []string{"Other", "M", "Open"} {
		if taken(n) {
			t.Errorf("%s should not be taken", n)
		}
	}
}
