package check

import (
	"strings"
	"testing"

	"alder/internal/diag"
)

// A page's query parameter may name its query key (`electionId: Text = "" as "ElectionId"`,
// #55). The key must be one a URL carries as written, unique among the page's keys (a plain
// parameter's key is its name), and only a query parameter takes one: a `{segment}` has no key.

// wirePage is a page module Home whose page declares params (the text inside `{ … }`) at path.
func wirePage(path, params string) map[string]string {
	return map[string]string{
		"App.alder": appModule("", "n: Int = 0"),
		"Home.alder": "module Home\n\npage \"" + path + "\" { " + params + " }\n\ntype Model = { }\n\n" +
			"load : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\n" +
			"view : Model -> Session -> Html Msg\nview model session = <p>x</p>\n",
	}
}

func TestPageWireNames(t *testing.T) {
	wantClean(t, checkApp(t, wirePage("/r/{cfg}", `cfg: Text, electionId: Text = "" as "ElectionId", contestId: Maybe Int as "ContestId"`)))
	wantClean(t, checkApp(t, wirePage("/r", `id: Text = "" as "id"`))) // the key may be the name itself
	wantClean(t, checkApp(t, wirePage("/r", `a: Text = "" as "a-b.c_d~e9"`)))
	for _, c := range []struct{ path, params, want string }{
		{"/r", `e: Text = "" as ""`, "the wire name of page parameter `e` is empty"},
		{"/r", `e: Text = "" as "Election Id"`, "the wire name `Election Id` of page parameter `e` is not a plain query key: use letters, digits, `-`, `_`, `.` and `~`"},
		{"/r", `e: Text = "" as "a&b"`, "the wire name `a&b` of page parameter `e` is not a plain query key"},
		{"/r", `e: Text = "" as "_p.x"`, "the wire name `_p.x` of page parameter `e` starts with `_p.`, which Alder keeps for message URLs"},
		{"/r", `a: Text = "" as "K", b: Text = "" as "K"`, "page parameters `a` and `b` both read the query key `K`"},
		{"/r", `a: Text = "", b: Text = "" as "a"`, "page parameters `a` and `b` both read the query key `a`"},
		{"/r", `b: Text = "" as "a", a: Text = ""`, "page parameters `b` and `a` both read the query key `a`"},
		{"/r/{cfg}", `cfg: Text as "Cfg"`, "page parameter `cfg` is the path segment `{cfg}`, not a query parameter: only a query parameter takes a wire name (`as \"Cfg\"`)"},
	} {
		bag := checkApp(t, wirePage(c.path, c.params))
		if got := codes(bag); len(got) != 1 || got[0] != diag.InvalidDecl || !strings.Contains(render(bag), c.want) {
			t.Errorf("%s: want one E0011 containing %q, got:\n%s", c.params, c.want, render(bag))
		}
	}
}
