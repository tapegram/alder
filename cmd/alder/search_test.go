package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runSearch runs `alder search args` with dir as the working directory. Tests using it change the
// process's working directory and so must not be parallel.
func runSearch(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	var out, errb bytes.Buffer
	code := runSearchTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

func searchJSONOf(t *testing.T, out string) searchJSON {
	t.Helper()
	var r searchJSON
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return r
}

func findRow(r searchJSON, module, name string) *searchRowJSON {
	for i, row := range r.Results {
		if row.Module == module && row.Name == name {
			return &r.Results[i]
		}
	}
	return nil
}

func TestSearch_StdlibOutsideApp(t *testing.T) {
	code, out, errb := runSearch(t, t.TempDir(), "split")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb)
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Text.split") {
			found = true
			if !strings.Contains(line, "Text -> Text -> List Text") || !strings.Contains(line, "stdlib") {
				t.Errorf("row = %q", line)
			}
		}
	}
	if !found {
		t.Errorf("no Text.split row:\n%s", out)
	}
}

func TestSearch_JSON(t *testing.T) {
	code, out, errb := runSearch(t, t.TempDir(), "--json", ":", "List", "Text")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb)
	}
	r := searchJSONOf(t, out)
	if r.Query != ": List Text" || r.Mode != "produces" {
		t.Errorf("query %q, mode %q", r.Query, r.Mode)
	}
	row := findRow(r, "Text", "split")
	if row == nil {
		t.Fatalf("no Text.split in %s", out)
	}
	if row.Args == nil || *row.Args != 2 || row.Match != "exact" || row.Source != "stdlib" {
		t.Errorf("row = %+v", row)
	}
	var raw struct{ Results []map[string]any }
	json.Unmarshal([]byte(out), &raw)
	for _, m := range raw.Results {
		if m["name"] == "split" {
			if _, ok := m["file"]; ok {
				t.Errorf("stdlib row has a file key: %v", m)
			}
		}
	}
}

func TestSearch_JSONEmpty(t *testing.T) {
	code, out, errb := runSearch(t, t.TempDir(), "--json", "zzzzqq")
	if code != 0 || !strings.Contains(out, `"results":[]`) {
		t.Errorf("code %d, stdout %q", code, out)
	}
	if !strings.Contains(errb, "no results") {
		t.Errorf("stderr %q", errb)
	}
}

func TestSearch_Limit(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := runSearch(t, dir, "--limit", "2", "Text.")
	if rows := strings.Count(strings.TrimSpace(out), "\n"); code != 0 || rows != 2 {
		t.Errorf("code %d, %d result rows:\n%s", code, rows, out)
	}
	for _, args := range [][]string{{"--limit", "x", "a"}, {"--limit"}, {"--limit", "0", "a"}, {"--limit", "-1", "a"}} {
		code, out, errb := runSearch(t, dir, args...)
		if code != 2 || out != "" || !strings.Contains(errb, searchUsage) {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, out, errb)
		}
	}
}

func TestSearch_Usage(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{nil, {"--nope", "a"}, {"--json"}} {
		code, _, errb := runSearch(t, dir, args...)
		if code != 2 || !strings.Contains(errb, searchUsage) {
			t.Errorf("%v: code %d, stderr %q", args, code, errb)
		}
	}
}

func TestSearch_BadType(t *testing.T) {
	code, out, errb := runSearch(t, t.TempDir(), "Text ->")
	if code != 2 || out != "" || !strings.Contains(errb, "not a complete type") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errb)
	}
}

func TestSearch_InApp(t *testing.T) {
	src, err := filepath.Abs("../../examples/blog")
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), "blog")
	copyDir(t, src, app)
	var rows []searchRowJSON
	for _, dir := range []string{app, filepath.Join(app, "app")} {
		code, out, errb := runSearch(t, dir, "--json", "notFound")
		if code != 0 {
			t.Fatalf("%s: code %d, stderr %q", dir, code, errb)
		}
		row := findRow(searchJSONOf(t, out), "App", "notFound")
		if row == nil {
			t.Fatalf("%s: no App.notFound in %s", dir, out)
		}
		if row.Source != "app" || !strings.HasPrefix(row.File, "app/") || row.Line <= 0 {
			t.Errorf("%s: row = %+v", dir, row)
		}
		rows = append(rows, *row)
	}
	if rows[0] != rows[1] {
		t.Errorf("rows differ: %+v vs %+v", rows[0], rows[1])
	}
}

func TestSearch_AppBroken(t *testing.T) {
	check := func(label, app string) {
		code, out, errb := runSearch(t, app, "--json", "split")
		if code != 0 {
			t.Fatalf("%s: code %d, stderr %q", label, code, errb)
		}
		r := searchJSONOf(t, out)
		if findRow(r, "Text", "split") == nil {
			t.Errorf("%s: no Text.split in %s", label, out)
		}
		if !strings.Contains(errb, "alder search: the app does not type-check, so its declarations are left out; run alder check") {
			t.Errorf("%s: stderr %q", label, errb)
		}
	}
	typeErr, err := filepath.Abs("../../testdata/corpus/E0002-own-module")
	if err != nil {
		t.Fatal(err)
	}
	blog, err := filepath.Abs("../../examples/blog")
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "broken")
	copyDir(t, typeErr, broken)
	check("type error", broken)

	// A clean app plus one file that does not parse, so only the parse failure is in play.
	clean := filepath.Join(t.TempDir(), "blog")
	copyDir(t, blog, clean)
	if code, _, errb := runSearch(t, clean, "--json", "split"); code != 0 || strings.Contains(errb, "does not type-check") {
		t.Fatalf("clean blog: code %d, stderr %q", code, errb)
	}
	if err := os.WriteFile(filepath.Join(clean, "app", "Bad.alder"), []byte("module Bad\n\nx = = =\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	check("parse error", clean)
}

func TestSearch_HolesAreFine(t *testing.T) {
	app := t.TempDir()
	if err := os.MkdirAll(filepath.Join(app, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "module App\n\nerror : Error -> Html\nerror err = <h1>Something went wrong</h1>\n\nnotFound : Html\nnotFound = <h1>Not found</h1>\n\nhelper : Int -> Int\nhelper n = n + 1\n\nunfinished : Int\nunfinished = ?x\n"
	if err := os.WriteFile(filepath.Join(app, "app", "App.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errb := runSearch(t, app, "--json", "helper")
	if code != 0 {
		t.Fatalf("code %d, stderr %q", code, errb)
	}
	row := findRow(searchJSONOf(t, out), "App", "helper")
	if row == nil || row.Source != "app" {
		t.Errorf("row = %+v in %s (stderr %q)", row, out, errb)
	}
}
