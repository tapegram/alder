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

// helperApp is an app with a file page (Sites.MnSos), a fake (Gate), test helpers in their own
// module (Helpers) and a test in another module that uses them.
func helperApp() map[string]string {
	return map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html Never\nerror err = <html><body><h1>Error</h1></body></html>\n\nnotFound : Html Never\nnotFound = <html><body><h1>Not found</h1></body></html>\n",
		"Sites/MnSos.alder": "module Sites.MnSos\n\npage \"/mnsos\" { cfg: Text }\n\ntype Model = { cfg: Text }\n\n" +
			"load : Params -> Session -> Query Model\nload params session = Query.succeed { cfg: params.cfg }\n\n" +
			"view : Model -> Session -> File\nview model session = File.text { contentType: \"text/plain\", body: model.cfg }\n",
		"Gate.alder": "module Gate\n\ntype OpenedId = brand Text\n\ncapability Gate {\n  open : Task Bool,\n}\n\nfake Gate {\n  table Opened {\n    id: OpenedId @id,\n  }\n  open = Task.succeed True\n}\n",
		"Helpers.alder": "module Helpers\n\nimport Gate\n\n" +
			"fetchText : Text -> Task { status: Int, body: Text }\nfetchText cfg = {\n  let _ <- Test.fake Gate\n  let r <- Test.fetch (Sites.MnSos { cfg: cfg })\n  Task.succeed { status: r.status, body: Bytes.toText r.body ?? \"\" }\n}\n\n" +
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

// boardPage is a page module with the given wired values.
func boardPage(load, update, view string) string {
	return "module Board\n\nimport Helpers\n\npage \"/board\" { }\n\ntype Model = { n: Int }\n\nmsg Refresh\n\n" + load + "\n\n" + update + "\n\n" + view + "\n"
}

const (
	boardLoad   = "load : Params -> Session -> Query Model\nload params session = Query.succeed { n: 0 }"
	boardUpdate = "update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = Task.succeed Rerender"
	boardView   = "view : Model -> Session -> Html Msg\nview model session = <button click={Refresh}>Refresh</button>"
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
			boardPage(boardLoad, "update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = {\n  let r <- Helpers.fetchText \"x\"\n  Task.succeed Rerender\n}", boardView),
			"E0040 Board.alder:16:12 `Helpers.fetchText` is a test helper (it uses `Test.fake`): only a `test` or another test helper may use it"},
		{"a chain", "Board.alder",
			boardPage(boardLoad, "refresh : Task Unit\nrefresh = Task.map (\\r -> ()) Helpers.at400\n\nupdate : Msg -> Model -> Session -> Task Nav\nupdate msg model session = {\n  let _ <- refresh\n  Task.succeed Rerender\n}", boardView),
			"E0040 Board.alder:19:12 `refresh` is a test helper (it uses `Helpers.at400`, a test helper): only a `test` or another test helper may use it"},
		{"a page's update", "Board.alder",
			boardPage(boardLoad, "update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = {\n  let r <- Test.fetch (Sites.MnSos { cfg: \"x\" })\n  Task.succeed Rerender\n}", boardView),
			"E0040 Board.alder:16:12 `Test.fetch` is only available in a `test` or a test helper: `update` is the page's update, not a helper"},
		{"a page's view", "Board.alder",
			boardPage(boardLoad, boardUpdate, "view : Model -> Session -> Html Msg\nview model session = <p>{Helpers.answerName}</p>"),
			"E0040 Board.alder:18:26 `Helpers.answerName` is a test helper (it uses `Shown`): only a `test` or another test helper may use it"},
		{"a page's view using Doc", "Board.alder",
			boardPage(boardLoad, boardUpdate, "view : Model -> Session -> Html Msg\nview model session = {\n  let d : Maybe Doc = Nothing\n  <p>x</p>\n}"),
			"E0040 Board.alder:19:17 `Doc` is only available in a `test` or a test helper: `view` is the page's view, not a helper"},
		{"App's values", "App.alder",
			"module App\n\nimport Helpers\n\nerror : Error -> Html Never\nerror err = <html><body><h1>{Helpers.answerName}</h1></body></html>\n\nnotFound : Html Never\nnotFound = <html><body><h1>Not found</h1></body></html>\n",
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
