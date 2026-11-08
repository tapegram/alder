package ir

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

var wsRun = regexp.MustCompile(`\s+`)

// spaces reads every run of white space in s as one space.
func spaces(s string) string { return wsRun.ReplaceAllString(s, " ") }

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
	if r, ok := p.Choose["org"].(*Route); !ok || r.Module != "Orgs.Index" {
		t.Errorf("choose: %v", p.Choose)
	}
	stores := map[string]string{}
	for _, r := range p.Routes {
		if r.Module == "Admin.Projects" && (r.Silo != "org" || strings.Join(r.Requires, ",") != "user") {
			t.Errorf("admin form: requires %v silo %q", r.Requires, r.Silo)
		}
		if r.Method != "GET" {
			stores[r.Module+"."+r.Name] = r.Store.String()
		}
	}
	want := map[string]string{"Projects.Index.Add": "silo org", "Projects.Index.Seed": "silo org", "Projects.Index.Both": "silo org",
		"Projects.Index.Tag": "shared", "Notes.Index.Add": "shared", "Orgs.Index.Pick": "shared", "Admin.Projects.Remove": "silo org"}
	for k, w := range want {
		if stores[k] != w {
			t.Errorf("arm %s: store %q, want %q", k, stores[k], w)
		}
	}
	if stores["App.Leave"] != "shared" {
		t.Errorf("App.Leave: store %q", stores["App.Leave"])
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

// TestIR_RouteSilos (#160): the corpus app silo-routes-ok lowers `choose org` naming a GET route,
// each write route's store, the admin form `silo org` on routes, and the narrowed sessions the
// routes of one module share.
func TestIR_RouteSilos(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/silo-routes-ok")
	if r, ok := p.Choose["org"].(*Route); !ok || r.Module != "Orgs" || r.Name != "Pick" || r.TargetPath() != "/orgs" {
		t.Errorf("choose: %v", p.Choose)
	}
	got := map[string]string{}
	for _, r := range p.Routes {
		got[r.Module+"."+r.Name] = fmt.Sprintf("%s %s silo=%q session=%s", r.Method, r.Store, r.Silo, r.Session.Name)
	}
	want := map[string]string{
		"Login.Login":           `GET shared silo="" session=Session`,
		"Orgs.Pick":             `GET shared silo="" session=Pick.Session`,
		"Projects.Index":        `GET shared silo="" session=Index.Session`,
		"Projects.Add":          `POST silo org silo="" session=Index.Session`,
		"Admin.Projects.Manage": `GET shared silo="org" session=Manage.Session`,
		"Admin.Projects.Remove": `DELETE silo org silo="org" session=Manage.Session`,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("routes:\n got %v\nwant %v", got, want)
	}
}
