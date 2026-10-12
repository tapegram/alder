package main

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDist_OutOfTree is the distribution test (M10): the alder binary builds, tests and serves
// an app with nothing of the repository but itself. It builds the binary, copies each example
// (without its build/) to a temporary directory outside the checkout, and runs `alder check`,
// `alder build` and `alder test` there with a PATH holding only the binary's directory and Go's,
// HOME a fresh directory, and the Go caches (module, build) the real ones, so a warm machine
// stays offline. Nothing written may name the checkout: the generated go.mod has no absolute
// path and no file under build/ contains the checkout's path. Then it starts the built app and
// GETs `/`.
func TestDist_OutOfTree(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	if testing.Short() {
		t.Skip("builds and tests both examples")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	goenv := map[string]string{}
	out, err := exec.Command("go", "env", "GOROOT", "GOMODCACHE", "GOPATH", "GOCACHE", "GOPROXY").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	for i, v := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		goenv[[]string{"GOROOT", "GOMODCACHE", "GOPATH", "GOCACHE", "GOPROXY"}[i]] = v
	}

	tmp := t.TempDir()
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real
	}
	if rel, err := filepath.Rel(repo, tmp); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("the temporary directory %s is inside the checkout %s", tmp, repo)
	}
	bin := filepath.Join(tmp, "bin")
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "alder"), ".")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build of alder: %v\n%s", err, out)
	}
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + filepath.Join(goenv["GOROOT"], "bin"),
		"HOME=" + home,
		"TMPDIR=" + os.TempDir(),
		"GOMODCACHE=" + goenv["GOMODCACHE"],
		"GOPATH=" + goenv["GOPATH"],
		"GOCACHE=" + goenv["GOCACHE"],
		"GOPROXY=" + goenv["GOPROXY"],
		"GOTOOLCHAIN=local",
	}

	for _, name := range []string{"todos", "blog"} {
		t.Run(name, func(t *testing.T) {
			work := filepath.Join(tmp, "work")
			app := filepath.Join(work, name)
			if err := os.CopyFS(app, os.DirFS(filepath.Join(repo, "examples", name))); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(app, "build")); err != nil { // the checkout's own builds
				t.Fatal(err)
			}
			alder := func(args ...string) string {
				t.Helper()
				cmd := exec.Command(filepath.Join(bin, "alder"), args...)
				cmd.Dir, cmd.Env = app, append(env, "PWD="+app)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("alder %s: %v\n%s", strings.Join(args, " "), err, out)
				}
				return string(out)
			}
			alder("check")
			if out := alder("build"); out != "wrote build/"+name+"/"+name+"\n" {
				t.Errorf("alder build printed %q", out)
			}
			if out := alder("test"); !strings.Contains(out, " passed") {
				t.Errorf("alder test printed:\n%s", out)
			}

			gomod, err := os.ReadFile(filepath.Join(app, "build", name, "go.mod"))
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(gomod), "\n") {
				if f := strings.Fields(line); len(f) > 0 && f[0] == "replace" && line != "replace alder => ./rt" {
					t.Errorf("go.mod: %q, want replace alder => ./rt", line)
				}
				if strings.Contains(line, "/Users/") || strings.Contains(line, "/home/") || strings.Contains(line, tmp) {
					t.Errorf("go.mod names an absolute path: %q", line)
				}
			}
			// nothing under build/ names the checkout (in either spelling of its path)
			needles := [][]byte{[]byte(repo)}
			if real, err := filepath.EvalSymlinks(repo); err == nil && real != repo {
				needles = append(needles, []byte(real))
			}
			err = filepath.WalkDir(filepath.Join(app, "build"), func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for _, n := range needles {
					if bytes.Contains(b, n) {
						t.Errorf("%s contains the checkout's path %s", path, n)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			// the built app serves /
			r := startAppNamed(t, app, name, "ALDER_DEV=1", "ALDER_SECRET=")
			c := &http.Client{Timeout: 10 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			res, err := c.Get("http://" + r.addr + "/")
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusSeeOther {
				t.Fatalf("GET / = %d:\n%s", res.StatusCode, body)
			}
			if code := r.stop(t, os.Interrupt); code != 0 {
				t.Fatalf("after SIGINT: exit %d; stderr:\n%s", code, r.stderr.String())
			}
		})
	}

	// alder version from the same binary
	cmd := exec.Command(filepath.Join(bin, "alder"), "version")
	cmd.Env = env
	if out, err := cmd.Output(); err != nil || !strings.HasPrefix(string(out), "alder ") {
		t.Errorf("alder version: %v %q", err, out)
	}
}
