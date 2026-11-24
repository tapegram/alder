package migrate

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"alder/internal/gen"
	"alder/rt/db"
)

// SiloDecls are the app's `silo` declarations as the generated Silos var has them (gen's
// silosVar): silo-ok's `silo org { Project }`, Ref the partition column's @ref.
func TestSiloDecls(t *testing.T) {
	p := programIR(t, "../../testdata/corpus/silo-ok")
	got := SiloDecls(p)
	want := []db.Silo{{Key: "org", Tables: []string{"Project.Project"}, Ref: "Org.Org"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SiloDecls = %+v, want %+v", got, want)
	}
	files, err := gen.Emit(p, gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, b := range files {
		all.Write(b)
	}
	var items []string
	for _, s := range got {
		var ts []string
		for _, tb := range s.Tables {
			ts = append(ts, strconv.Quote(tb))
		}
		items = append(items, "{Key: "+strconv.Quote(s.Key)+", Tables: []string{"+strings.Join(ts, ", ")+"}, Ref: "+strconv.Quote(s.Ref)+"}")
	}
	if line := "var Silos = []db.Silo{" + strings.Join(items, ", ") + "}\n"; !strings.Contains(all.String(), line) {
		t.Errorf("gen writes no %s", line)
	}
	if SiloDecls(programIR(t, "../../examples/blog")) != nil {
		t.Errorf("an app without silo declares some")
	}
}
