package rt

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestFiles_Complete: Files holds exactly the runtime's files in the repository (`git ls-files
// rt`, untracked files not ignored included, so a file not yet added counts): every one but the
// tests, testdata and embed.go itself. A file added to rt/ without being listed in embed.go
// fails here, before a user's build misses it.
func TestFiles_Complete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "--full-name", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	var want []string
	for _, f := range strings.Fields(string(out)) {
		f = strings.TrimPrefix(f, "rt/")
		if strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "testdata/") || strings.Contains(f, "/testdata/") || f == "embed.go" {
			continue
		}
		if _, err := os.Stat(f); err != nil { // deleted in the worktree, not yet in the index
			continue
		}
		want = append(want, f)
	}
	var got []string
	if err := fs.WalkDir(Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			got = append(got, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	slices.Sort(got)
	for _, f := range want {
		if !slices.Contains(got, f) {
			t.Errorf("rt/%s is not embedded: add it to the //go:embed lines of rt/embed.go", f)
		}
	}
	for _, f := range got {
		if !slices.Contains(want, f) {
			t.Errorf("rt/%s is embedded but is not a runtime file of the repository", f)
		}
	}
}

// TestDistModule: the runtime's module files (dist/go.mod.txt, dist/go.sum.txt) are the
// repository's go.mod and go.sum, whose requirements are all the runtime's: every module the
// root go.mod requires is one a package of rt/ imports, directly or not, so the written module
// lists only its real dependencies. When a dependency changes, copy the files again:
//
//	cp go.mod rt/dist/go.mod.txt && cp go.sum rt/dist/go.sum.txt
//
// A dependency of the compiler alone would make the second check fail: then the dist files
// need their own derivation, no longer a copy.
func TestDistModule(t *testing.T) {
	for _, f := range [][2]string{{"../go.mod", "dist/go.mod.txt"}, {"../go.sum", "dist/go.sum.txt"}} {
		root, err := os.ReadFile(f[0])
		if err != nil {
			t.Fatal(err)
		}
		dist, err := Files.ReadFile(f[1])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(root, dist) {
			t.Errorf("rt/%s differs from %s: cp go.mod rt/dist/go.mod.txt && cp go.sum rt/dist/go.sum.txt", f[1], strings.TrimPrefix(f[0], "../"))
		}
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	// The runtime's dependencies differ by platform (modernc.org/libc uses go-isatty and
	// go-strftime on darwin, not on linux), and an app is built wherever alder runs: a module is
	// used if some release target (.goreleaser.yaml) uses it.
	var used []string
	for _, target := range [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./...")
		cmd.Env = append(os.Environ(), "GOOS="+target[0], "GOARCH="+target[1], "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list (%s/%s): %v", target[0], target[1], err)
		}
		used = append(used, strings.Fields(string(out))...)
	}
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	inBlock := false
	for _, line := range strings.Split(string(mod), "\n") {
		f := strings.Fields(line)
		var req string
		switch {
		case len(f) == 2 && f[0] == "require" && f[1] == "(":
			inBlock = true
		case len(f) > 0 && f[0] == ")":
			inBlock = false
		case len(f) >= 3 && f[0] == "require":
			req = f[1]
		case inBlock && len(f) >= 2:
			req = f[0]
		}
		if req != "" && !slices.Contains(used, req) {
			t.Errorf("go.mod requires %s, which no package of rt/ uses: the runtime's module would list a dependency it does not have", req)
		}
	}
}

// TestDesignSystemVersion: the vendored `alder-ui` snapshot (al/, written by
// scripts/vendor-al.sh <tag>) is one release: VERSION names the tag, the manifest's `version`
// field and the stylesheet's banner say the same.
func TestDesignSystemVersion(t *testing.T) {
	v, err := Files.ReadFile("al/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	tag := strings.TrimSpace(string(v))
	if !strings.HasPrefix(tag, "v") || strings.Contains(tag, "placeholder") {
		t.Fatalf("al/VERSION is %q, want a release tag", tag)
	}
	manifest, err := Files.ReadFile("al/custom-elements.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != tag {
		t.Errorf("al/custom-elements.json says version %q, al/VERSION %q", m.Version, tag)
	}
	css, err := Files.ReadFile("al/al.css")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(css, []byte("/*! alder-ui "+tag+" ")) {
		t.Errorf("al/al.css begins %.40q, want the banner of alder-ui %s", css, tag)
	}
}
