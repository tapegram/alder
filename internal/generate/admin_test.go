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
		got := adminNeeds(c.s)
		if !strings.HasPrefix(got, AdminNeeds) || strings.TrimPrefix(got, AdminNeeds) != c.want {
			t.Errorf("adminNeeds(%+v) = %q, want …%q", c.s, got, c.want)
		}
	}
}
