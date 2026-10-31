package fill

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/generate"
)

const fixture = "../../testdata/generate/fixture"

// holeApp copies the generate fixture to a temporary directory with Home.update.Submit's last
// line a hole `?done`, a value `stay : Task Nav` with a comment above `update`, and the extra
// files (path relative to the app → text). It returns the app's directory.
func holeApp(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	err := filepath.WalkDir(fixture, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(fixture, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		if strings.HasSuffix(p, ".db") || rel == "build" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	src := strings.Replace(string(b), "    Task.succeed (Redirect (Home {}))\n", "    ?done\n", 1)
	src = strings.Replace(src, "update : Msg", "stay : Task Nav\nstay = Task.succeed Rerender\n\n-- What the page does with a message.\n-- Submit writes the post.\nupdate : Msg", 1)
	os.WriteFile(home, []byte(src), 0o644)
	for rel, text := range extra {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	return dir
}

// loadHole loads the app in dir and finds the hole address names.
func loadHole(t *testing.T, dir, address, hole string) (*generate.Program, generate.Hole) {
	t.Helper()
	p, err := generate.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	h, err := generate.FindHole(p, address, hole)
	if err != nil {
		t.Fatal(err)
	}
	return p, *h
}

func TestPrompt_Contents(t *testing.T) {
	dir := holeApp(t, nil)
	p, h := loadHole(t, dir, "Home.update.Submit", "")
	got := Prompt(p, h)
	for _, want := range []string{
		"Fill the typed hole ?done in update.Submit of module Home (app/Home.alder).\nIt must have type: Task Nav\n",
		"The comment above the declaration:\n-- What the page does with a message.\n-- Submit writes the post.\n",
		"The module, app/Home.alder:\n```alder\nmodule Home\n",
		"    ?done\n", // the hole as written
		"Names in scope (those whose type fits the hole first):\nstay : Task Nav\n",
		"p : ", // the arm's binder
		"type Post.Status = Draft | Published",
		"module Task\n",
		"module Db\n",
		"module Prelude\n",
		"\nReply with the expression only.\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt has no %q:\n%s", want, got)
		}
	}
	// fits first: stay (Task Nav) before load, though load sorts first
	scope := got[strings.Index(got, "Names in scope"):]
	if i, j := strings.Index(scope, "stay : Task Nav"), strings.Index(scope, "\nload : "); i < 0 || j < 0 || i > j {
		t.Errorf("a fitting name is not first:\n%s", got)
	}
	for _, not := range []string{
		`<al-page width="narrow">`,       // Layout's body
		"test \"a new post is a draft\"", // Table.Post's tests
	} {
		if strings.Contains(got, not) {
			t.Errorf("the prompt has %q", not)
		}
	}
	if !strings.HasSuffix(got, "Reply with the expression only.\n") {
		t.Errorf("the prompt does not end with the instruction")
	}
}

func TestPrompt_CoreStdAlwaysIncluded(t *testing.T) {
	// a module that has not written `List.` or `Text.` anywhere
	dir := holeApp(t, map[string]string{"app/Bare.alder": "module Bare\n\nanswer : Int\nanswer = ?n\n"})
	p, h := loadHole(t, dir, "Bare.answer", "")
	got := Prompt(p, h)
	for _, mod := range []string{"List", "Maybe", "Result", "Text", "Int", "Float", "Dict"} {
		if !strings.Contains(got, "module "+mod+"\n") {
			t.Errorf("the prompt has no module %s", mod)
		}
	}
	if strings.Contains(got, "module Json\n") {
		t.Errorf("the prompt has Json, which nothing names")
	}
}

func TestPrompt_TestHoleIncludesTest(t *testing.T) {
	dir := holeApp(t, nil)
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	os.WriteFile(home, []byte(strings.Replace(string(b), "Task.succeed (r.answer == Redirected (Auth.Login {}))", "Task.succeed ?expected", 1)), 0o644)
	p, h := loadHole(t, dir, `Home."a visitor is sent to log in"`, "")
	got := Prompt(p, h)
	for _, want := range []string{"It must have type: Bool", "module Test\n", "r : { answer: Answer"} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt has no %q:\n%s", want, got)
		}
	}
}

func TestPrompt_RetryCarriesErrors(t *testing.T) {
	got := RetryTurn("app/Home.alder:30:5: error[E0003]: expected Task Nav, got Task Unit\n")
	want := "The checker refused it:\napp/Home.alder:30:5: error[E0003]: expected Task Nav, got Task Unit\nReply with a corrected expression only."
	if got != want {
		t.Errorf("RetryTurn = %q", got)
	}
}

func TestStripFence(t *testing.T) {
	for in, want := range map[string]string{
		"Task.succeed Rerender":                           "Task.succeed Rerender",
		"  Task.succeed Rerender \n\n":                    "Task.succeed Rerender",
		"```alder\nTask.succeed Rerender\n```":            "Task.succeed Rerender",
		"```\n{\n  let x <- a\n  b\n}\n```\n":             "{\n  let x <- a\n  b\n}",
		"```Task.succeed Rerender```":                     "Task.succeed Rerender",
		"```alder\n```inner```\n```":                      "```inner```",
		"Task.succeed (r.answer == `x`)":                  "Task.succeed (r.answer == `x`)",
		"```alder\nTask.succeed Rerender\n```\nand prose": "```alder\nTask.succeed Rerender\n```\nand prose",
	} {
		if got := StripFence(in); got != want {
			t.Errorf("StripFence(%q) = %q, want %q", in, got, want)
		}
	}
}
