package mutate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Query.redirect is a guard (#8): `if ok then Query.redirect p else Query.succeed m` gives one
// "guard dropped" mutant, the redirect arm dropped, and the line it shows names
// `Query.redirect …`.
func TestRedirectIsGuard(t *testing.T) {
	dir := t.TempDir()
	srcs := map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html\nerror err = <h1>Error</h1>\n\nnotFound : Html\nnotFound = <h1>Not found</h1>\n",
		"Home.alder": `module Home

route Home = GET "/" { go: Bool = False }

Home.load : Home.Params -> Session -> Query Bool
Home.load params session = if params.go then Query.redirect (Other.Other {}) else Query.succeed True

Home.view : Bool -> Session -> Html
Home.view ok session = <p>home</p>
`,
		"Other.alder": `module Other

route Other = GET "/other" { }

Other.load : Other.Params -> Session -> Query Int
Other.load params session = Query.succeed 1

Other.view : Int -> Session -> Html
Other.view n session = <p>other</p>
`,
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range srcs {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ms, _ := Collect(lower(t, dir))
	var guards []Mutant
	for _, m := range ms {
		if m.Kind == GuardDropped {
			guards = append(guards, m)
		}
	}
	if len(guards) != 1 || guards[0].After != "the redirect arm dropped" || !strings.Contains(guards[0].Before, "then Query.redirect (Other.Other {}) else") {
		t.Fatalf("guard mutants: %+v", guards)
	}
}
