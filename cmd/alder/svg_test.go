package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// svgChart is a route module whose view draws a chart in SVG (#306), with an app test that reads
// the page's shapes.
const svgChart = `module Chart

route Chart = GET "/chart" { }

Chart.load : Chart.Params -> Session -> Query { n: Int }
Chart.load params session = Query.succeed { n: 1 }

Chart.view : { n: Int } -> Session -> Html
Chart.view model session =
  <svg viewBox="0 0 10 10" role="img">
    <title>t</title>
    <polyline points="0,0 5,5" fill="none" stroke="currentColor" stroke-width="2" />
    <line x1="0" y1="0" x2="10" y2="0" />
  </svg>

test "the chart draws its line" = {
  let page <- Test.get (Chart {})
  Task.succeed (
    { lines: Test.count "svg polyline" page.doc, box: Test.attr "viewBox" "svg" page.doc }
      == { lines: 1, box: Just "0 0 10 10" }
  )
}
`

// An SVG view renders with `viewBox` in its case and each shape closed, and an app test counts
// its shapes (#306).
func TestSVG_ViewAndAppTest(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := filepath.Join(t.TempDir(), "todos")
	copyDir(t, todosDir(t), dir)
	if err := os.WriteFile(filepath.Join(dir, "app", "Chart.alder"), []byte(svgChart), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTest(t, dir, "-run", "chart")
	if code != 0 || !strings.Contains(stdout, `ok   Chart "the chart draws its line"`) {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	want := `<svg viewBox="0 0 10 10" role="img"><title>t</title><polyline points="0,0 5,5" fill="none" stroke="currentColor" stroke-width="2"></polyline><line x1="0" y1="0" x2="10" y2="0"></line></svg> : Html` + "\n"
	in := `<svg viewBox="0 0 10 10" role="img"><title>t</title><polyline points="0,0 5,5" fill="none" stroke="currentColor" stroke-width="2" /><line x1="0" y1="0" x2="10" y2="0" /></svg>`
	if code, stdout, stderr := runEval(t, "", dir, in); code != 0 || stdout != want {
		t.Fatalf("eval: exit %d\nstdout: %q\nwant:   %q\nstderr: %s", code, stdout, want, stderr)
	}
}
