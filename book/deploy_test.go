package book

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fences are the fenced blocks of the Markdown file at path whose info string starts with
// lang: their info strings and sources, in order.
func fences(t *testing.T, path, lang string) (infos, sources []string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	in, mine := false, false
	var src strings.Builder
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "```") {
			if !in {
				in = true
				info := strings.TrimPrefix(l, "```")
				mine = info == lang || strings.HasPrefix(info, lang+" ")
				if mine {
					infos = append(infos, info)
				}
				src.Reset()
				continue
			}
			if mine {
				sources = append(sources, src.String())
			}
			in, mine = false, false
			continue
		}
		if in && mine {
			src.WriteString(l + "\n")
		}
	}
	return infos, sources
}

// TestBook_DeployDeclaration: every `deploy { … }` fragment of chapter 16 is a declaration
// `alder check` accepts in App: each is appended to the scaffold's App (it has none) in a copy
// of the scaffold, and the app must check without an error; a fragment with a `-- E0030`
// comment is the chapter's refused secret, and must be refused with E0030's secret message.
func TestBook_DeployDeclaration(t *testing.T) {
	bin := alder(t)
	infos, sources := fences(t, "16-deploy.md", "alder")
	n := 0
	for i, src := range sources {
		if infos[i] != "alder fragment" || !strings.HasPrefix(src, "deploy {") {
			continue
		}
		n++
		dir := filepath.Join(t.TempDir(), "app")
		if err := copyApp(scaffoldApp, dir); err != nil {
			t.Fatal(err)
		}
		app := filepath.Join(dir, "app", "App.alder")
		b, err := os.ReadFile(app)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(app, append(b, []byte("\n"+src)...), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(bin, "check", dir).CombinedOutput()
		if strings.Contains(src, "-- E0030") {
			if err == nil || !strings.Contains(string(out), "error[E0030]: a secret belongs in the environment, not in `App`: name it in `requires`") {
				t.Errorf("book/16-deploy.md: deploy fragment %d is not refused with E0030's secret message: %v\n%s", n, err, out)
			}
			continue
		}
		if err != nil {
			t.Errorf("book/16-deploy.md: deploy fragment %d does not check in App: %v\n%s\n%s", n, err, src, out)
		}
	}
	if n < 3 {
		t.Errorf("chapter 16 has %d deploy fragments, want the declaration's, the examples', the backup's and the preview's", n)
	}
}

// TestBook_CIGolden: a ```yaml golden=<path> block of chapter 16 is, byte for byte, that file
// (the golden workflow `alder ci init`'s tests compare its output with), so the chapter shows
// the file the command writes.
func TestBook_CIGolden(t *testing.T) {
	infos, sources := fences(t, "16-deploy.md", "yaml")
	n := 0
	for i, info := range infos {
		path, ok := strings.CutPrefix(info, "yaml golden=")
		if !ok {
			continue
		}
		n++
		want, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("book/16-deploy.md: golden=%s: %v", path, err)
			continue
		}
		if sources[i] != string(want) {
			t.Errorf("book/16-deploy.md: the golden=%s block is not that file:\n%s", path, firstDiff(sources[i], string(want)))
		}
	}
	if n == 0 {
		t.Error("chapter 16 has no ```yaml golden= block")
	}
}
