package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"alder/internal/generate"
	"alder/internal/theme"
)

const themeFixtures = "../../internal/theme/testdata/import/"

// themeImport runs `alder theme import args… --dir dir`, with stdin (when not "") as os.Stdin.
func themeImport(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	if stdin != "" {
		f := filepath.Join(t.TempDir(), "stdin")
		if err := os.WriteFile(f, []byte(stdin), 0o644); err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		old := os.Stdin
		os.Stdin = in
		defer func() { os.Stdin = old }()
	}
	var out, errb bytes.Buffer
	code := runThemeTo(append(append([]string{"import"}, args...), "--dir", dir), &out, &errb)
	return code, out.String(), errb.String()
}

func readAppAlder(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "app", "App.alder"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestThemeImport_Writes(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeImport(t, dir, "", themeFixtures+"shadcn-v4.css")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if !strings.Contains(readAppAlder(t, dir), "primary:") {
		t.Errorf("App.alder has no theme:\n%s", readAppAlder(t, dir))
	}
	first := strings.SplitN(out, "\n", 2)[0]
	if !regexp.MustCompile(`^theme: \d+ tokens → app/App.alder  \(light \d+, dark \d+, theme-wide \d+\)`).MatchString(first) {
		t.Errorf("first line %q", first)
	}
	if strings.Contains(first, "replaced") {
		t.Errorf("nothing was replaced: %q", first)
	}
}

func TestThemeImport_ReplacedSummary(t *testing.T) {
	dir := tempAppAbs(t, "../../examples/blog", "blog")
	// the blog sets radius and light/dark primary; the export below sets only `--secondary`
	css := ":root { --secondary: oklch(0.97 0 0); }\n.dark { --secondary: oklch(0.27 0 0); }\n"
	code, out, errb := themeImport(t, dir, css, "-")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	first := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasSuffix(first, "; replaced the theme that was there)") {
		t.Errorf("first line %q", first)
	}
	for _, f := range []string{"radius", "light.primary", "dark.primaryForeground"} {
		if !strings.Contains(out, f) {
			t.Errorf("dropped fields do not list %s:\n%s", f, out)
		}
	}
}

func TestThemeImport_UnplacedBlock(t *testing.T) {
	dir := genApp(t, "scaffold")
	src, err := os.ReadFile(themeFixtures + "tweakcn.css")
	if err != nil {
		t.Fatal(err)
	}
	_, un, sk, err := theme.Import(string(src))
	if err != nil || len(un) == 0 || sk.N == 0 {
		t.Fatalf("fixture: %d unplaced, %d skipped, %v", len(un), sk.N, err)
	}
	code, out, errb := themeImport(t, dir, "", themeFixtures+"tweakcn.css")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if !strings.Contains(out, "not placed — paste into static/app.css if you want them:\n") {
		t.Errorf("no unplaced heading:\n%s", out)
	}
	var want strings.Builder
	for _, l := range strings.Split(strings.TrimRight(theme.UnplacedCSS(un), "\n"), "\n") {
		want.WriteString(strings.TrimRight("  "+l, " ") + "\n")
	}
	if !strings.Contains(out, want.String()) {
		t.Errorf("output lacks the indented UnplacedCSS:\n%s\nwant\n%s", out, want.String())
	}
	if !regexp.MustCompile(`(?m)^skipped \d+ blocks? \(.+\)$`).MatchString(out) {
		t.Errorf("no skipped line:\n%s", out)
	}
}

func TestThemeImport_Print(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeImport(t, dir, "", themeFixtures+"shadcn-v4.css", "--print")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	src, _ := os.ReadFile(themeFixtures + "shadcn-v4.css")
	th, _, _, _ := theme.Import(string(src))
	want, err := generate.ThemeText(th)
	if err != nil {
		t.Fatal(err)
	}
	if out != want+"\n" {
		t.Errorf("stdout:\n%s\nwant\n%s", out, want)
	}
	if !strings.HasPrefix(out, "theme {") {
		t.Errorf("not a declaration: %s", out)
	}
	if readAppAlder(t, dir) != before {
		t.Error("--print changed App.alder")
	}
}

func TestThemeImport_DryRun(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeImport(t, dir, "", themeFixtures+"shadcn-v4.css", "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if readAppAlder(t, dir) != before {
		t.Error("--dry-run changed App.alder")
	}
	if !strings.Contains(out, "would edit app/App.alder") || !strings.Contains(out, "dry run: nothing written") {
		t.Errorf("output:\n%s", out)
	}
}

func TestThemeImport_Stdin(t *testing.T) {
	dir := genApp(t, "scaffold")
	src, _ := os.ReadFile(themeFixtures + "shadcn-v3.css")
	code, out, errb := themeImport(t, dir, string(src), "-")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if !strings.Contains(readAppAlder(t, dir), "primary:") {
		t.Errorf("App.alder has no theme:\n%s", readAppAlder(t, dir))
	}
}

func TestThemeImport_NoTheme(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeImport(t, dir, "", themeFixtures+"garbage.css")
	if code != 1 {
		t.Fatalf("exit %d, want 1: %s%s", code, out, errb)
	}
	if !strings.Contains(errb, "no theme in `garbage.css`") {
		t.Errorf("stderr: %s", errb)
	}
	if readAppAlder(t, dir) != before {
		t.Error("App.alder changed")
	}
	code, _, errb = themeImport(t, dir, "nothing here", "-")
	if code != 1 || !strings.Contains(errb, "no theme in `stdin`") {
		t.Errorf("stdin: exit %d: %s", code, errb)
	}
}

func TestThemeImport_JSON(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeImport(t, dir, "", themeFixtures+"tweakcn.css", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, k := range []string{"written", "file", "theme", "unplaced", "skipped", "diagnostics"} {
		if _, ok := got[k]; !ok {
			t.Errorf("no key %q in %s", k, out)
		}
	}
	var un []map[string]string
	if err := json.Unmarshal(got["unplaced"], &un); err != nil || len(un) == 0 || un[0]["block"] == "" || un[0]["reason"] == "" {
		t.Errorf("unplaced: %s (%v)", got["unplaced"], err)
	}
	var th theme.Theme
	if err := json.Unmarshal(got["theme"], &th); err != nil || len(th.Light) == 0 {
		t.Errorf("theme: %s (%v)", got["theme"], err)
	}
	if string(got["written"]) != "true" || string(got["file"]) != `"app/App.alder"` {
		t.Errorf("written %s, file %s", got["written"], got["file"])
	}

	// nothing written: exit 1, written false
	code, out, _ = themeImport(t, dir, "", themeFixtures+"garbage.css", "--json")
	if code != 1 || !strings.Contains(out, `"written":false`) {
		t.Errorf("garbage: exit %d: %s", code, out)
	}
}

func TestHelpJSON_ListsThemeImport(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := writeHelpJSON(&out); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Commands []struct {
			Name        string
			Subcommands []struct {
				Name  string
				Args  []string
				Flags []struct{ Name string }
			}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Commands {
		if c.Name != "theme" {
			continue
		}
		for _, s := range c.Subcommands {
			if s.Name == "import" {
				var flags []string
				for _, f := range s.Flags {
					flags = append(flags, f.Name)
				}
				if len(s.Args) != 1 || s.Args[0] != "<file.css>" || !strings.Contains(strings.Join(flags, " "), "--print") {
					t.Errorf("args %v, flags %v", s.Args, flags)
				}
				return
			}
		}
	}
	t.Fatalf("`theme import` is not in help --json:\n%s", out.String())
}
