package check

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// checkFiles checks an app made of files (path under app/ → source) and returns its Result and
// its error diagnostics as "Code file:line:col message".
func checkFiles(t *testing.T, files map[string]string) (*Result, []string) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, "app", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, bag := runDirResult(t, dir)
	var out []string
	for _, d := range bag.Items {
		if d.Severity == "error" {
			out = append(out, fmt.Sprintf("%s %s:%d:%d %s", d.Code, filepath.Base(d.Span.File), d.Span.Start.Line, d.Span.Start.Col, d.Message))
		}
	}
	return res, out
}

// helperApp is an app with a file route (Sites.MnSos.MnSos), a fake (Gate), test helpers in their own
// module (Helpers) and a test in another module that uses them.
func helperApp() map[string]string {
	return map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html\nerror err = <html><body><h1>Error</h1></body></html>\n\nnotFound : Html\nnotFound = <html><body><h1>Not found</h1></body></html>\n",
		"Sites/MnSos.alder": "module Sites.MnSos\n\nroute MnSos = GET \"/mnsos\" { cfg: Text }\n\n" +
			"MnSos.load : MnSos.Params -> Session -> Query Text\nMnSos.load params session = Query.succeed params.cfg\n\n" +
			"MnSos.view : Text -> Session -> File\nMnSos.view cfg session = File.text { contentType: \"text/plain\", body: cfg }\n",
		"Gate.alder": "module Gate\n\ntype OpenedId = brand Text\n\ncapability Gate {\n  open : Task Bool,\n}\n\nfake Gate {\n  table Opened {\n    id: OpenedId @id,\n  }\n  open = Task.succeed True\n}\n",
		"Helpers.alder": "module Helpers\n\nimport Gate\n\n" +
			"fetchText : Text -> Task { status: Int, body: Text }\nfetchText cfg = {\n  let _ <- Test.fake Gate\n  let r <- Test.fetch (Sites.MnSos.MnSos { cfg: cfg })\n  Task.succeed { status: r.status, body: Bytes.toText r.body ?? \"\" }\n}\n\n" +
			"at400 : Task { status: Int, body: Text }\nat400 = fetchText \"final\"\n\n" +
			"heading : Doc -> Text\nheading doc = Test.text \"h1\" doc ?? \"\"\n\n" +
			"shown : Answer -> Bool\nshown a = a == Shown\n\n" +
			"opened : Query Int\nopened = Db.from Gate.Opened |> Db.count\n\n" +
			"answerName : Text\nanswerName = if shown Shown then \"shown\" else \"other\"\n\n" +
			"plain : Int\nplain = 1\n",
		"Checks.alder": "module Checks\n\nimport Helpers\n\n" +
			"test \"at minute 400 the file is the final\" = {\n  let r <- Helpers.at400\n  Task.succeed (r.status == 200 && Text.contains \"final\" r.body)\n}\n\n" +
			"test \"shown\" = Helpers.shown Shown && Helpers.answerName == \"shown\"\n",
	}
}

// boardPage is a route module (Board, GET, and Refresh, POST) with the given wired values.
func boardPage(load, update, view string) string {
	return "module Board\n\nimport Helpers\n\nroute Board   = GET  \"/board\"   { }\nroute Refresh = POST \"/refresh\" { }\n\n" + load + "\n\n" + update + "\n\n" + view + "\n"
}

const (
	boardLoad   = "Board.load : Board.Params -> Session -> Query Int\nBoard.load params session = Query.succeed 0"
	boardUpdate = "Refresh.update : Refresh.Params -> Session -> Task Response\nRefresh.update params session = Task.succeed Response.refresh"
	boardView   = "Board.view : Int -> Session -> Html\nBoard.view n session = <button click={Refresh}>Refresh</button>"
)

// A value that uses something only tests may use is a test helper (#4): tests anywhere may use
// it, production code may not, and some values can never be one.
func TestTestHelpers(t *testing.T) {
	with := func(extra map[string]string) map[string]string {
		files := helperApp()
		for k, v := range extra {
			files[k] = v
		}
		return files
	}
	t.Run("accepted", func(t *testing.T) {
		res, got := checkFiles(t, helperApp())
		if len(got) != 0 {
			t.Fatalf("unexpected: %q", got)
		}
		want := map[string]string{
			"Helpers.fetchText":  "`Test.fake`",
			"Helpers.at400":      "`fetchText`, a test helper",
			"Helpers.heading":    "`Doc`",
			"Helpers.shown":      "`Answer`",
			"Helpers.opened":     "`Opened`",
			"Helpers.answerName": "`Shown`",
		}
		for k, v := range want {
			if res.TestOnly[k] != v {
				t.Errorf("TestOnly[%s] = %q, want %q", k, res.TestOnly[k], v)
			}
		}
		if len(res.TestOnly) != len(want) {
			t.Errorf("TestOnly = %v", res.TestOnly)
		}
	})
	cases := []struct{ name, file, src, want string }{
		{"production use refused with the reason", "Board.alder",
			boardPage(boardLoad, "Refresh.update : Refresh.Params -> Session -> Task Response\nRefresh.update params session = {\n  let r <- Helpers.fetchText \"x\"\n  Task.succeed Response.refresh\n}", boardView),
			"E0040 Board.alder:13:12 `Helpers.fetchText` is a test helper (it uses `Test.fake`): only a `test` or another test helper may use it"},
		{"a chain", "Board.alder",
			boardPage(boardLoad, "refresh : Task Unit\nrefresh = Task.map (\\r -> ()) Helpers.at400\n\nRefresh.update : Refresh.Params -> Session -> Task Response\nRefresh.update params session = {\n  let _ <- refresh\n  Task.succeed Response.refresh\n}", boardView),
			"E0040 Board.alder:16:12 `refresh` is a test helper (it uses `Helpers.at400`, a test helper): only a `test` or another test helper may use it"},
		{"a route's update", "Board.alder",
			boardPage(boardLoad, "Refresh.update : Refresh.Params -> Session -> Task Response\nRefresh.update params session = {\n  let r <- Test.fetch (Sites.MnSos.MnSos { cfg: \"x\" })\n  Task.succeed Response.refresh\n}", boardView),
			"E0040 Board.alder:13:12 `Test.fetch` is only available in a `test` or a test helper: `Refresh.update` is the route's update, not a helper"},
		{"a route's view", "Board.alder",
			boardPage(boardLoad, boardUpdate, "Board.view : Int -> Session -> Html\nBoard.view n session = <p>{Helpers.answerName}</p>"),
			"E0040 Board.alder:15:28 `Helpers.answerName` is a test helper (it uses `Shown`): only a `test` or another test helper may use it"},
		{"a route's view using Doc", "Board.alder",
			boardPage(boardLoad, boardUpdate, "Board.view : Int -> Session -> Html\nBoard.view n session = {\n  let d : Maybe Doc = Nothing\n  <p>x</p>\n}"),
			"E0040 Board.alder:16:17 `Doc` is only available in a `test` or a test helper: `Board.view` is the route's view, not a helper"},
		{"App's values", "App.alder",
			"module App\n\nimport Helpers\n\nerror : Error -> Html\nerror err = <html><body><h1>{Helpers.answerName}</h1></body></html>\n\nnotFound : Html\nnotFound = <html><body><h1>Not found</h1></body></html>\n",
			"E0040 App.alder:6:30 `Helpers.answerName` is a test helper (it uses `Shown`): only a `test` or another test helper may use it"},
		{"a fake's entry", "Stripe.alder",
			"module Stripe\n\ncapability Stripe {\n  charge : Task Bool,\n}\n\nfake Stripe {\n  charge = {\n    let _ <- Test.fake Stripe\n    Task.succeed True\n  }\n}\n",
			"E0040 Stripe.alder:9:14 in `fake Stripe`'s `charge`: `Test.fake` is only available in a `test` or a test helper: a fake's entry is not a helper"},
		{"a field default", "Opts.alder",
			"module Opts\n\ntype Opts = { trace: Task (List Text) = Test.trace }\n",
			"E0040 Opts.alder:3:41 `Test.trace` is only available in a `test` or a test helper: a field default is not a helper"},
		{"a brand check", "Slug.alder",
			"module Slug\n\ntype Slug = brand Text where (\\s -> Test.has s Test.trace)\n",
			"E0040 Slug.alder:3:37 `Test.has` is only available in a `test` or a test helper: a brand check is not a helper"},
		{"Doc in a type stays E0003", "Snap.alder",
			"module Snap\n\ntype Snap = { doc: Doc }\n",
			"E0003 Snap.alder:3:20 `Doc` is a test type: it exists only inside a `test`"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, got := checkFiles(t, with(map[string]string{c.file: c.src}))
			if len(got) == 0 || got[0] != c.want {
				t.Fatalf("got %q\nwant first %q", got, c.want)
			}
		})
	}
}
