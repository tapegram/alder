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
				if len(s.Args) != 1 || s.Args[0] != "<file.css | file.json>" || !strings.Contains(strings.Join(flags, " "), "--print") {
					t.Errorf("args %v, flags %v", s.Args, flags)
				}
				return
			}
		}
	}
	t.Fatalf("`theme import` is not in help --json:\n%s", out.String())
}

func TestThemeImport_PrintJSONIsUsageError(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeImport(t, dir, "", themeFixtures+"shadcn-v4.css", "--print", "--json")
	if code != 2 || out != "" || !strings.Contains(errb, "choose one of --print and --json") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errb)
	}
}

func TestThemeImport_PrintUnplacedToStderr(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeImport(t, dir, "", themeFixtures+"tweakcn.css", "--print")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if !strings.HasPrefix(out, "theme {") || strings.Contains(out, "not placed") {
		t.Errorf("stdout should be the declaration alone:\n%s", out)
	}
	if !strings.Contains(errb, "not placed — paste into static/app.css") || !strings.Contains(errb, "--font-serif") {
		t.Errorf("stderr lacks the unplaced block:\n%s", errb)
	}
}

func TestThemeImport_BadInput(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, _, errb := themeImport(t, dir, "", filepath.Join(t.TempDir(), "nope.css"))
	if code != 1 || !strings.Contains(errb, "cannot read") || !strings.Contains(errb, "nope.css") {
		t.Errorf("missing file: exit %d: %s", code, errb)
	}
	code, _, errb = themeImport(t, dir, strings.Repeat("a", 300<<10), "-")
	if code != 1 || !strings.Contains(errb, "256 KiB") {
		t.Errorf("too big: exit %d: %s", code, errb)
	}
}

func TestThemeImport_JSONGarbageHasAllKeys(t *testing.T) {
	dir := genApp(t, "scaffold")
	_, out, _ := themeImport(t, dir, "", themeFixtures+"garbage.css", "--json")
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, k := range []string{"written", "file", "theme", "unplaced", "skipped", "diagnostics"} {
		if _, ok := got[k]; !ok {
			t.Errorf("no key %q in %s", k, out)
		}
	}
}

// aliasCycleCSS is an export whose two colours alias each other: the write is refused (E0042).
const aliasCycleCSS = ":root { --primary: var(--secondary); --secondary: var(--primary) }"

func TestThemeImport_RefusedKeepsDiagnostics(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeImport(t, dir, aliasCycleCSS, "-")
	if code != 1 || !strings.Contains(out, "E0042") {
		t.Errorf("text: exit %d:\n%s%s", code, out, errb)
	}
	code, out, _ = themeImport(t, dir, aliasCycleCSS, "-", "--json")
	var got struct {
		Diagnostics []struct{ Code string }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 1 || len(got.Diagnostics) == 0 || got.Diagnostics[0].Code != "E0042" {
		t.Errorf("json: exit %d (%v):\n%s", code, err, out)
	}
	if readAppAlder(t, dir) != before {
		t.Errorf("App.alder changed")
	}
}

const dtcgOne = `{"color":{"$type":"color","primary":{"$value":{"colorSpace":"srgb","components":[0.2,0.4,0.8]}}}}`

func copyPaper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := "../../internal/theme/presets/paper"
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestThemeImport_DTCGResolver(t *testing.T) {
	dir := genApp(t, "scaffold")
	tmp := copyPaper(t)
	code, out, errb := themeImport(t, dir, "", filepath.Join(tmp, "theme.resolver.json"))
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if !strings.Contains(out, "(light 32, dark 32, theme-wide 15") {
		t.Errorf("summary: %s", out)
	}
}

func TestThemeImport_DTCGSingleFile(t *testing.T) {
	dir := genApp(t, "scaffold")
	f := filepath.Join(t.TempDir(), "x.tokens.json")
	if err := os.WriteFile(f, []byte(dtcgOne), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errb := themeImport(t, dir, "", f)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if !strings.Contains(readAppAlder(t, dir), "primary:") {
		t.Errorf("App.alder:\n%s", readAppAlder(t, dir))
	}
}

func TestThemeImport_DTCGStdin(t *testing.T) {
	dir := genApp(t, "scaffold")
	if code, out, errb := themeImport(t, dir, dtcgOne, "-"); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	dir = genApp(t, "scaffold")
	resolver, err := os.ReadFile(filepath.Join(copyPaper(t), "theme.resolver.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, errb := themeImport(t, dir, string(resolver), "-")
	if code != 1 || !strings.Contains(errb, "a resolver needs its files: run `alder theme import <dir>/theme.resolver.json`") {
		t.Errorf("resolver on stdin: exit %d: %s", code, errb)
	}
}

func TestThemeImport_FormatForced(t *testing.T) {
	dir := genApp(t, "scaffold")
	f := filepath.Join(t.TempDir(), "x.tokens.json")
	if err := os.WriteFile(f, []byte(dtcgOne), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errb := themeImport(t, dir, "", f, "--format", "css")
	if code != 1 || !strings.Contains(errb, "no theme in `x.tokens.json`") {
		t.Errorf("css on json: exit %d: %s", code, errb)
	}
	code, _, errb = themeImport(t, dir, "", themeFixtures+"shadcn-v4.css", "--format", "dtcg")
	if code != 1 || !strings.Contains(errb, "JSON") && !strings.Contains(errb, "json") {
		t.Errorf("dtcg on css: exit %d: %s", code, errb)
	}
	code, _, errb = themeImport(t, dir, "", f, "--format", "yaml")
	if code != 1 || !strings.Contains(errb, "`css` or `dtcg`") {
		t.Errorf("bad format: exit %d: %s", code, errb)
	}
}

func TestThemeImport_DTCGNothingPlaced(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, _, errb := themeImport(t, dir, `{"nope":{"$type":"color","$value":"#fff"}}`, "-")
	if code != 1 || !strings.Contains(errb, "no token of alder-ui") || strings.Contains(errb, ":root") {
		t.Errorf("exit %d: %s", code, errb)
	}
	if readAppAlder(t, dir) != before {
		t.Error("App.alder changed")
	}
}

// themeUse runs `alder theme use args… --dir dir`.
func themeUse(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runThemeTo(append(append([]string{"use"}, args...), "--dir", dir), &out, &errb)
	return code, out.String(), errb.String()
}

func TestThemeUse_List(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeUse(t, dir)
	if code != 0 || errb != "" {
		t.Fatalf("exit %d: %s", code, errb)
	}
	want := "starting themes (alder-ui " + theme.Version() + "):\n" +
		"  zinc        Tools, dashboards, admin — the default, the safe choice.\n" +
		"  brutal      Launch pages, portfolios, anything that should shout.\n" +
		"  catppuccin  Developer-facing apps.\n" +
		"  paper       Reading: blogs, docs, long-form.\n" +
		"  pebble      Consumer, mobile-feel apps.\n" +
		"alder theme use <name> writes one into app/App.alder; the app owns it from there.\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestThemeUse_ListJSON(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeUse(t, dir, "--json")
	if code != 0 || errb != "" {
		t.Fatalf("exit %d: %s", code, errb)
	}
	var got struct {
		Version string `json:"version"`
		Presets []struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Purpose string `json:"purpose"`
		} `json:"presets"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Version != theme.Version() {
		t.Errorf("version %q, want %q", got.Version, theme.Version())
	}
	ps := theme.Presets()
	if len(got.Presets) != len(ps) {
		t.Fatalf("%d presets, want %d", len(got.Presets), len(ps))
	}
	for i, p := range ps {
		g := got.Presets[i]
		if g.Name != p.Name || g.Title != p.Title || g.Purpose != p.Purpose {
			t.Errorf("preset %d: got %+v, want %s/%s/%s", i, g, p.Name, p.Title, p.Purpose)
		}
	}
	if readAppAlder(t, dir) != before {
		t.Error("App.alder changed")
	}
}

func TestThemeUse_WritesAndReplaces(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeUse(t, dir, "paper")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if want := "theme: paper → app/App.alder  (light 32, dark 32, theme-wide 15)\n"; !strings.HasPrefix(out, want) {
		t.Errorf("out %q, want prefix %q", out, want)
	}
	p, _ := theme.PresetNamed("paper")
	text, err := generate.ThemeText(p.Theme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readAppAlder(t, dir), text) {
		t.Errorf("App.alder lacks the paper declaration:\n%s", readAppAlder(t, dir))
	}
	code, out, errb = themeUse(t, dir, "pebble")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if want := "theme: pebble → app/App.alder  (light 32, dark 32, theme-wide 15; replaced the theme that was there)\n"; !strings.HasPrefix(out, want) {
		t.Errorf("out %q", out)
	}
	if strings.Contains(out, "dropped") || strings.Contains(out, "tokens") {
		t.Errorf("import-only wording in %q", out)
	}
}

func TestThemeUse_PrintDryRun(t *testing.T) {
	dir := genApp(t, "scaffold")
	before := readAppAlder(t, dir)
	code, out, errb := themeUse(t, dir, "paper", "--print")
	if code != 0 || !strings.HasPrefix(out, "theme {") || errb != "" || readAppAlder(t, dir) != before {
		t.Errorf("print: exit %d out %q err %q", code, out, errb)
	}
	code, out, errb = themeUse(t, dir, "paper", "--dry-run")
	if code != 0 || readAppAlder(t, dir) != before || !strings.Contains(out, "theme: paper") {
		t.Errorf("dry-run: exit %d out %q err %q", code, out, errb)
	}
}

func TestThemeUse_JSON(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeUse(t, dir, "paper", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, k := range []string{"written", "file", "theme", "unplaced", "skipped", "diagnostics", "preset"} {
		if _, ok := got[k]; !ok {
			t.Errorf("no key %q in %s", k, out)
		}
	}
	if string(got["preset"]) != `"paper"` {
		t.Errorf("preset %s", got["preset"])
	}
}

func TestThemeUse_Unknown(t *testing.T) {
	dir := genApp(t, "scaffold")
	code, out, errb := themeUse(t, dir, "papr")
	if code != 1 {
		t.Errorf("exit %d", code)
	}
	all := out + errb
	if !strings.Contains(all, "no starting theme `papr`; did you mean `paper`?") || !strings.Contains(all, "starting themes (alder-ui") {
		t.Errorf("out %q err %q", out, errb)
	}
}

func TestThemeUse_AppDoesNotParse(t *testing.T) {
	dir := genApp(t, "scaffold")
	path := filepath.Join(dir, "app", "App.alder")
	broken := readAppAlder(t, dir) + "\nthis is ((( not alder\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errb := themeUse(t, dir, "paper")
	if code == 0 || readAppAlder(t, dir) != broken {
		t.Errorf("exit %d, file changed=%v: %s%s", code, readAppAlder(t, dir) != broken, out, errb)
	}
	if out+errb == "" {
		t.Error("no message")
	}
}

func TestHelpJSON_ThemeUse(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := writeHelpJSON(&out); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Commands []struct {
			Name        string
			Subcommands []struct{ Name string }
		}
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, c := range got.Commands {
		for _, s := range c.Subcommands {
			if c.Name == "theme" && s.Name == "use" {
				return
			}
		}
	}
	t.Fatal("`theme use` is not in help --json")
}

func TestThemeImport_JSONDarkLeftoverScheme(t *testing.T) {
	dir := genApp(t, "scaffold")
	src, err := filepath.Abs("../../internal/theme/testdata/dtcg/foreign-names")
	if err != nil {
		t.Fatal(err)
	}
	code, out, errb := themeImport(t, dir, "", filepath.Join(src, "theme.resolver.json"), "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	var got struct {
		Unplaced []struct{ Block, Scheme string } `json:"unplaced"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	var dark, other int
	for _, u := range got.Unplaced {
		if strings.HasPrefix(u.Block, "dark.tokens.json ") && u.Scheme == "dark" {
			dark++
		} else if u.Scheme != "" {
			other++
		}
	}
	if dark == 0 || other != 0 || !strings.Contains(out, `"scheme":"dark"`) {
		t.Errorf("scheme missing or misplaced: %s", out)
	}
}
