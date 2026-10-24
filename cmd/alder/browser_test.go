//go:build browser

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The project's real-browser smoke test (M13), behind the build tag `browser`:
//
//	go test -tags browser -run Browser ./cmd/alder
//
// It serves the blog with `alder run` and drives a headless Chromium through
// testdata/browser/smoke.cjs (Playwright): a boosted click loads no document and changes the
// URL and the title; Back and Forward restore the scroll position; the login form's redirect
// is morphed in with the session cookie; a link to a missing page morphs the 404 page in; a
// slow post followed by a click ends on the click's page; a link to a file under /static/ is a
// plain navigation. M7b adds the admin's delete dialog across a body morph (its counts, its own
// Remove, its link to the Delete page), a 404 and a 500 with the dev panel (History, Copy as
// curl), a bulk delete's toast and its timeout, the panel after the delete's redirect, the search
// of a long reference table, the Edit page of a row whose reference sorts past the 50 offered, the
// focus after a refused message, and the interactive elements (menu, popover, tabs, tooltip, an
// author-written dialog) on an Alder page, testdata/browser/Smoke.alder, added to the copy of the
// blog it serves; the 500 renames a table in the dev database (its path is the script's second
// argument).
//
// It needs node and Playwright with its Chromium. Playwright is looked for in
// $ALDER_PLAYWRIGHT_DIR, else $HOME/.cache/alder-playwright; when it is missing the test is
// skipped, unless ALDER_BROWSER_INSTALL=1 is set, which installs it there with npm
// (testdata/browser/package.json: a download from the npm registry, so never implicit).
// Chromium is Playwright's own (`npx playwright install chromium`). Without them the test is
// skipped, saying why.
func TestBrowser_BoostedNavigation(t *testing.T) {
	t.Parallel()
	node, env := playwrightEnv(t)
	script, err := filepath.Abs(filepath.Join("testdata", "browser", "smoke.cjs"))
	if err != nil {
		t.Fatal(err)
	}

	smokePage, err := os.ReadFile(filepath.Join("testdata", "browser", "Smoke.alder"))
	if err != nil {
		t.Fatal(err)
	}
	smokeFile, err := os.ReadFile(filepath.Join("testdata", "browser", "SmokeFile.alder"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := startRunAppWith(t, "blog", map[string]string{"app/Smoke.alder": string(smokePage), "app/SmokeFile.alder": string(smokeFile)})
	cmd := exec.Command(node, script, r.base, filepath.Join(filepath.Dir(r.dir), "build", "blog", "dev.db"))
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("smoke.cjs: %v\n%s\n%s", err, out, stderr.String())
	}
	var res struct {
		Checks []struct {
			Name string
			OK   bool
			Got  string
		}
	}
	if err := json.Unmarshal(out, &res); err != nil || len(res.Checks) == 0 {
		t.Fatalf("smoke.cjs printed %q (%v)\n%s", out, err, stderr.String())
	}
	for _, c := range res.Checks {
		if !c.OK {
			t.Errorf("%s: got %s", c.Name, c.Got)
		}
	}
	t.Logf("%d checks in %v", len(res.Checks), time.Since(start).Round(time.Millisecond))
}

// playwrightDir is where the test finds (or installs) Playwright.
func playwrightDir(t *testing.T) string {
	if d := os.Getenv("ALDER_PLAYWRIGHT_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory for the Playwright cache: set ALDER_PLAYWRIGHT_DIR")
	}
	return filepath.Join(home, ".cache", "alder-playwright")
}

// playwrightEnv finds node and Playwright with its Chromium (installing Playwright when
// ALDER_BROWSER_INSTALL=1), or skips the test, saying why. env is the environment to run a
// Playwright script with.
func playwrightEnv(t *testing.T) (node string, env []string) {
	t.Helper()
	var err error
	node, err = exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on PATH")
	}
	dir := playwrightDir(t)
	nodePath := filepath.Join(dir, "node_modules")
	if _, err := os.Stat(filepath.Join(nodePath, "playwright")); err != nil {
		if os.Getenv("ALDER_BROWSER_INSTALL") != "1" {
			t.Skip("Playwright is not installed in " + dir + ": set ALDER_BROWSER_INSTALL=1 to install it there with npm")
		}
		npm, err := exec.LookPath("npm")
		if err != nil {
			t.Skip("Playwright is not installed in " + dir + " and npm is not on PATH")
		}
		pkg, err := os.ReadFile(filepath.Join("testdata", "browser", "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), pkg, 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(npm, "install", "--no-audit", "--no-fund")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("installing Playwright into %s failed: %v\n%s", dir, err, out)
		}
	}
	env = append(os.Environ(), "NODE_PATH="+nodePath)
	probe := exec.Command(node, "-e", `process.stdout.write(require('playwright').chromium.executablePath())`)
	probe.Env = env
	exe, err := probe.Output()
	if err != nil {
		t.Skipf("Playwright does not load from %s: %v", nodePath, err)
	}
	if _, err := os.Stat(string(exe)); err != nil {
		t.Skipf("Playwright's Chromium is not installed (%s): run `NODE_PATH=%s npx playwright install chromium`", exe, nodePath)
	}
	return node, env
}
