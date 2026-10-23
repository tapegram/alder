//go:build browser

package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// browserScript is the absolute path of testdata/browser/<name>; startRun changes the working
// directory, so a test takes it first.
func browserScript(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "browser", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// runBrowserScript runs the Playwright script (browserScript) against r, with the app's
// directory as its second argument, and fails the test for each check that is not ok. See
// browser_test.go for what the browser tests need.
func runBrowserScript(t *testing.T, script string, r *devRun) {
	t.Helper()
	node, env := playwrightEnv(t)
	start := time.Now()
	cmd := exec.Command(node, script, r.base, r.dir)
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s\n%s\n%s", script, err, out, stderr.String(), r.errs.String())
	}
	var res struct {
		Checks []struct {
			Name string
			OK   bool
			Got  string
		}
	}
	if err := json.Unmarshal(out, &res); err != nil || len(res.Checks) == 0 {
		t.Fatalf("%s printed %q (%v)\n%s", script, out, err, stderr.String())
	}
	for _, c := range res.Checks {
		if !c.OK {
			t.Errorf("%s: got %s", c.Name, c.Got)
		}
	}
	t.Logf("%d checks in %v", len(res.Checks), time.Since(start).Round(time.Millisecond))
}

// A compile error shows in the dev panel: an Errors tab (first, with a count) and a strip fixed
// to the top of the viewport that moves nothing; a good build clears both.
func TestBrowser_ErrorsInPanel(t *testing.T) {
	t.Parallel()
	playwrightEnv(t) // skip before the build when there is no browser
	script := browserScript(t, "errors.cjs")
	runBrowserScript(t, script, startRun(t))
}

// The dev panel's top edge resizes it (120 px up to 80% of the viewport), a button or
// Ctrl+Shift+D minimises it to a small tab that reserves no space, and its state is remembered
// (localStorage) across a morph and a reload; with storage unavailable it still works.
func TestBrowser_PanelResizeAndMinimise(t *testing.T) {
	t.Parallel()
	playwrightEnv(t)
	script := browserScript(t, "panel.cjs")
	runBrowserScript(t, script, startRun(t))
}
