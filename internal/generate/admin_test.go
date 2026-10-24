package generate

import (
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
