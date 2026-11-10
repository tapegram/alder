package check

import (
	"strings"
	"testing"

	"alder/internal/diag"
)

// A route's query parameter may name its query key (`electionId: Text = "" as "ElectionId"`,
// #55). The key must be one a URL carries as written, unique among the route's keys (a plain
// parameter's key is its name), and only a query parameter takes one: a `{segment}` has no key.

// wirePage is a module Home whose GET route Home declares params (the text inside `{ … }`) at path.
func wirePage(path, params string) map[string]string {
	return map[string]string{
		"App.alder": appModule("", "n: Int = 0"),
		"Home.alder": "module Home\n\nroute Home = GET \"" + path + "\" { " + params + " }\n\n" +
			"Home.load : Home.Params -> Session -> Query Int\nHome.load params session = Query.succeed 1\n\n" +
			"Home.view : Int -> Session -> Html\nHome.view n session = <p>x</p>\n",
	}
}

func TestRouteWireNames(t *testing.T) {
	wantClean(t, checkApp(t, wirePage("/r/{cfg}", `cfg: Text, electionId: Text = "" as "ElectionId", contestId: Maybe Int as "ContestId"`)))
	wantClean(t, checkApp(t, wirePage("/r", `id: Text = "" as "id"`))) // the key may be the name itself
	wantClean(t, checkApp(t, wirePage("/r", `a: Text = "" as "a-b.c_d~e9"`)))
	wantClean(t, checkApp(t, wirePage("/r", `e: Text = "" as "_p.x"`))) // `_p.` is a plain query key (#199)
	for _, c := range []struct{ path, params, want string }{
		{"/r", `e: Text = "" as ""`, "the wire name of route parameter `e` is empty"},
		{"/r", `e: Text = "" as "Election Id"`, "the wire name `Election Id` of route parameter `e` is not a plain query key: use letters, digits, `-`, `_`, `.` and `~`"},
		{"/r", `e: Text = "" as "a&b"`, "the wire name `a&b` of route parameter `e` is not a plain query key"},
		{"/r", `a: Text = "" as "K", b: Text = "" as "K"`, "route parameters `a` and `b` both read the query key `K`"},
		{"/r", `a: Text = "", b: Text = "" as "a"`, "route parameters `a` and `b` both read the query key `a`"},
		{"/r", `b: Text = "" as "a", a: Text = ""`, "route parameters `b` and `a` both read the query key `a`"},
		{"/r/{cfg}", `cfg: Text, other: Text = "" as "cfg"`, "the wire name `cfg` of route parameter `other` is the path segment `{cfg}`: give `other` another wire name"},
		{"/r/{cfg}", `cfg: Text as "Cfg"`, "route parameter `cfg` is the path segment `{cfg}`, not a query parameter: only a query parameter takes a wire name (`as \"Cfg\"`)"},
	} {
		bag := checkApp(t, wirePage(c.path, c.params))
		if got := codes(bag); len(got) != 1 || got[0] != diag.InvalidDecl || !strings.Contains(render(bag), c.want) {
			t.Errorf("%s: want one E0011 containing %q, got:\n%s", c.params, c.want, render(bag))
		}
	}
}
