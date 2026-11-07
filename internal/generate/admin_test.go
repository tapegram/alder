package generate

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"alder/internal/types"
)

// TestAdminNeeds: the precondition's refusal names what the app lacks, and the user table only
// when there is one.
func TestAdminNeeds(t *testing.T) {
	role := &SessionField{Name: "role", Type: &types.Union{Module: "Account", Name: "Kind"}}
	user := &SessionField{Name: "user"}
	for _, c := range []struct {
		s    *Session
		want string
	}{
		{&Session{}, ": this app's session has no `user`; add a user table, a login that sets `user`, and a `role` (chapter 6, \"Rules\" and \"Roles\")"},
		{&Session{User: user, UserTable: "Member"}, ": add `type Role = Reader | Admin` and a column `role: Role = Reader` to `Member`, `role: Member.Role = Reader` to `session` in `App`, and set it at login (chapter 6)"},
		{&Session{User: user}, ": add a union with an `Admin` constructor and `role: Role = Reader` of it to `session` in `App`, and set it at login (chapter 6)"},
		{&Session{User: user, UserTable: "Member", Role: role}, ": add `Admin` to the session's role type `Account.Kind`, and set the role at login (chapter 6)"},
	} {
		got := adminNeeds(&Program{}, c.s)
		if !strings.HasPrefix(got, AdminNeeds) || strings.TrimPrefix(got, AdminNeeds) != c.want {
			t.Errorf("adminNeeds(%+v) = %q, want …%q", c.s, got, c.want)
		}
	}
}

// TestAdminPluralFolders: each table's pages live in a plural folder named by the table's name,
// whether the table is in `Table.*` (fixture) or at the top (fixture-silo's Activity).
func TestAdminPluralFolders(t *testing.T) {
	for _, c := range []struct {
		app  string
		want []string
		not  []string
	}{
		{"fixture", []string{"app/Admin/Posts/Index.alder", "app/Admin/Comments/Bulk.alder", "app/Admin/Tags/Show.alder"}, []string{"Admin/Table", "Admin/Post/", "Admin/Comment/"}},
		{"fixture-silo", []string{"app/Admin/Activities/Index.alder", "app/Admin/Projects/Index.alder"}, []string{"Admin/Table", "Admin/Activity/", "Admin/Project/"}},
	} {
		p, err := Load("../../testdata/generate/" + c.app)
		if err != nil {
			t.Fatal(err)
		}
		files, order := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
		for _, w := range c.want {
			if _, ok := files[w]; !ok {
				t.Errorf("%s: no file %s in %v", c.app, w, order)
			}
		}
		for _, path := range order {
			for _, n := range c.not {
				if strings.Contains(path, n) {
					t.Errorf("%s: path %s contains %s", c.app, path, n)
				}
			}
		}
	}
}

// TestAdminNoDuplicatePaths: no two pages of the admin share a file, including a silo's picker
// and dashboard next to its key table's own pages (Admin/Orgs/).
func TestAdminNoDuplicatePaths(t *testing.T) {
	p, err := Load("../../testdata/generate/fixture-silo")
	if err != nil {
		t.Fatal(err)
	}
	_, order := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
	seen := map[string]bool{}
	for _, path := range order {
		if seen[path] {
			t.Errorf("%s is written twice", path)
		}
		seen[path] = true
	}
	for _, w := range []string{"app/Admin/Orgs/Pick.alder", "app/Admin/Orgs/Dashboard.alder", "app/Admin/Orgs/Index.alder"} {
		if !seen[w] {
			t.Errorf("no %s", w)
		}
	}
}

// TestAdminLayoutPageStack: the admin's own frame (no app Layout) wraps the page in the stack,
// with the nav before <main>; with an app Layout, the children go straight into Layout.Page.
func TestAdminLayoutPageStack(t *testing.T) {
	p, err := Load("../../testdata/generate/fixture")
	if err != nil {
		t.Fatal(err)
	}
	p.Layout = false
	files, _ := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
	src, ok := files["app/Admin/Layout.alder"]
	if !ok {
		t.Fatalf("no app/Admin/Layout.alder in %d files", len(files))
	}
	if !strings.Contains(src, `<main><al-stack gap="lg">{props.children}</al-stack></main>`) {
		t.Errorf("no page stack in:\n%s", src)
	}
	if i, j := strings.Index(src, "<nav"), strings.Index(src, "<main>"); i < 0 || i > j {
		t.Errorf("nav at %d, main at %d:\n%s", i, j, src)
	}
	p.Layout = true
	files, _ = AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
	if src, ok = files["app/Admin/Layout.alder"]; !ok {
		t.Fatal("no app/Admin/Layout.alder with an app Layout")
	}
	if !strings.Contains(src, "{props.children}\n  </Layout.Page>") || strings.Contains(src, `al-stack gap="lg"`) {
		t.Errorf("app-Layout frame wrong:\n%s", src)
	}
}

// TestAdminWireNames (#63): when the admin's pages already name a query parameter's wire key
// (`q: Text = "" as "Q"`, as on regen after the author added it), the regenerated route line
// keeps it, and the GET forms and the list's filter selects and tests write the key the target
// route reads (the list's own `q` and filters, the bulk page's `ids`), not the field name.
func TestAdminWireNames(t *testing.T) {
	p, err := Load("../../testdata/generate/fixture")
	if err != nil {
		t.Fatal(err)
	}
	index, bulk := p.RouteModule("Admin.Posts.Index"), p.RouteModule("Admin.Posts.Bulk")
	if index != nil || bulk != nil {
		t.Fatal("the fixture has an admin already")
	}
	p.Routes = append(p.Routes,
		&Route{Module: "Admin.Posts.Index", Name: "Index", Method: "GET", Params: []*Param{{Name: "q", Wire: "Q"}, {Name: "status", Wire: "Status"}, {Name: "featured"}}},
		&Route{Module: "Admin.Posts.Bulk", Name: "Bulk", Method: "GET", Params: []*Param{{Name: "ids", Wire: "Ids"}}})
	files, _ := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
	src := files["app/Admin/Posts/Index.alder"]
	for _, want := range []string{
		`q: Text = "" as "Q"`,
		`status: Text = "" as "Status"`,
		`featured: Text = "", `,
		`name: "Status",`,
		`name: "featured",`,
		`<input type="search" name="Q" value={model.q} />`,
		`<input type="hidden" name="sort" value={model.sort} />`,
		`<input type="hidden" name="Ids" value={row.id} />`,
		`select[name='Status'] option[selected]`,
		`select[name=featured] option[selected]`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("Index lacks %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, `name="ids"`) || strings.Count(src, `name="Ids"`) != 2 {
		t.Errorf("Index: the checkbox and the page form should both write Ids:\n%s", src)
	}
}

// TestAdminFormsNoEmptySpan: the New and Edit pages emit no empty element into their card; an
// empty <span> there left a band of the card's gap (#43).
func TestAdminFormsNoEmptySpan(t *testing.T) {
	for _, app := range []string{"fixture", "fixture-silo"} {
		p, err := Load("../../testdata/generate/" + app)
		if err != nil {
			t.Fatal(err)
		}
		files, _ := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
		n := 0
		for path, src := range files {
			if !strings.HasSuffix(path, "/New.alder") && !strings.HasSuffix(path, "/Edit.alder") {
				continue
			}
			n++
			if strings.Contains(src, "<span></span>") {
				t.Errorf("%s/%s has an empty <span></span>", app, path)
			}
		}
		if n == 0 {
			t.Errorf("%s: no New or Edit pages", app)
		}
	}
}

// routeLineRe is a `route NAME = METHOD "path" …` line: its name and method.
var routeLineRe = regexp.MustCompile(`(?m)^route (\w+) += (GET|POST|PUT|PATCH|DELETE) +"([^"]*)"(.*)$`)

// TestAdminRoutes (#160): every page of the admin is a route module, written with routes and
// Response, never with `page`, `msg` or `Nav`. Each table's modules have the routes their job
// needs: the list a GET; New a GET and a POST with the form that redirects to the row; Edit a GET
// and a PUT that redirects; Show a GET; Delete a GET (the confirm page) and a DELETE that
// redirects to the list with a success toast; the bulk page a GET with POST routes (a form each
// for a set) that answer refresh. The write routes are named for their action, so none shares a
// name with a form of its module or its GET route.
func TestAdminRoutes(t *testing.T) {
	for _, app := range []string{"fixture", "fixture-silo"} {
		p, err := Load("../../testdata/generate/" + app)
		if err != nil {
			t.Fatal(err)
		}
		files, order := AdminFiles(p, AdminOptions{Module: "Admin", Path: "/admin"})
		for _, path := range order {
			src := files[path]
			code, _, _ := strings.Cut(src, RegionBegin+"tests\n")
			for _, old := range []string{"\npage ", "\nmsg ", "Task Nav", "Html Msg", "Redirect (", "Reject ", "Notice."} {
				if strings.Contains(code, old) {
					t.Errorf("%s/%s still has %q", app, path, strings.TrimSpace(old))
				}
			}
			if path == "app/Admin/Layout.alder" {
				continue
			}
			got := map[string]string{}
			var names []string
			for _, m := range routeLineRe.FindAllStringSubmatch(src, -1) {
				got[m[1]] = m[2] + m[4]
				names = append(names, m[1])
			}
			kind := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".alder")
			if len(names) == 0 || names[0] != kind || !strings.HasPrefix(got[kind], "GET") {
				t.Errorf("%s/%s: the first route is not GET %s: %v", app, path, kind, got)
			}
			for _, name := range names {
				if strings.Contains(src, "\nform "+name+" ") {
					t.Errorf("%s/%s: route %s shares its name with a form", app, path, name)
				}
			}
			wantWrites := map[string][]string{
				"Index": nil, "Show": nil, "Pick": nil, "Dashboard": nil,
				"New":    {"Create"},
				"Edit":   {"Save"},
				"Delete": {"Destroy"},
			}[kind]
			writes := slices.DeleteFunc(slices.Clone(names), func(n string) bool { return strings.HasPrefix(got[n], "GET") || strings.HasPrefix(n, "Search") })
			if kind != "Bulk" && !slices.Equal(writes, wantWrites) {
				t.Errorf("%s/%s: write routes %v, want %v", app, path, writes, wantWrites)
			}
			switch kind {
			case "New":
				if !strings.HasPrefix(got["Create"], "POST") || !strings.Contains(got["Create"], " form ") || !strings.Contains(src, "Task.succeed (Response.redirect (") {
					t.Errorf("%s/%s: Create is not a POST with a form that redirects: %q", app, path, got["Create"])
				}
			case "Edit":
				if !strings.HasPrefix(got["Save"], "PUT") || !strings.Contains(got["Save"], " form ") || !strings.Contains(src, "Response.redirect (") {
					t.Errorf("%s/%s: Save is not a PUT with a form that redirects: %q", app, path, got["Save"])
				}
			case "Delete":
				if !strings.HasPrefix(got["Destroy"], "DELETE") || !strings.Contains(src, "|> Response.success ") {
					t.Errorf("%s/%s: Destroy is not a DELETE that redirects with a success toast: %q", app, path, got["Destroy"])
				}
			case "Bulk":
				if len(writes) == 0 || !strings.Contains(src, "Response.refresh") {
					t.Errorf("%s/%s: no bulk write route answering refresh: %v", app, path, got)
				}
				for _, w := range writes {
					if !strings.HasPrefix(got[w], "POST") || (strings.HasPrefix(w, "BulkSet") && !strings.Contains(got[w], " form Set")) {
						t.Errorf("%s/%s: %s is not a POST (with its form, for a set): %q", app, path, w, got[w])
					}
				}
			}
		}
	}
}
