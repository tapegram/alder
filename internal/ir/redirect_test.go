package ir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// redirectApp is a page whose load redirects with Query.redirect: applied to a page value with a
// path parameter, and passed as a function value (lowered through a Lambda).
var redirectApp = map[string]string{
	"App.alder": `module App

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>not found</p>
`,
	"Builder.alder": `module Builder

page "/builder/{seed}" { seed: Text }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>builder</p>
`,
	"Start.alder": `module Start

page "/start" { go: Text = "" }

type Model = { }

load : Params -> Session -> Query Model
load params session =
  if params.go == "builder" then Query.redirect (Builder { seed: "7" })
  else if params.go == "maybe" then Maybe.withDefault (Query.succeed { }) (Maybe.map Query.redirect (Just (Builder { seed: "8" })))
  else Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>start</p>
`,
}

func writeApp(t *testing.T, srcs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range srcs {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Query.redirect lowers to EffRedirect, applied or as a value (a Lambda whose body is the node).
func TestIR_QueryRedirect(t *testing.T) {
	d := Dump(lowerDir(t, writeApp(t, redirectApp)))
	if strings.Count(d, "(redirect") != 2 {
		t.Fatalf("want two redirect nodes:\n%s", d)
	}
	i := strings.LastIndex(d, "(redirect")
	if !strings.Contains(d[max(0, i-200):i], "lambda") {
		t.Fatalf("Query.redirect as a value is not a Lambda over the node:\n%s", d[max(0, i-200):min(len(d), i+100)])
	}
}
