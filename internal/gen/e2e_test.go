package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The M3c end-to-end tests: an app is emitted into a temporary module, a test program from
// testdata (a .go.txt, so this package does not compile it) is written next to it as
// zz_e2e_test.go, and `go test` runs it there. The program serves the app in-process (Serve +
// httptest) and drives it like a browser; see the programs for the scenarios.

// TestE2E_Todos runs the to-do app's scenario (testdata/e2e_todos_test.go.txt), steps 1–13: log
// in, add, toggle (a Datastar patch; without JavaScript a 303 back), show, delete, another user's
// messages (404), a tampered cookie, a failing capability (500, rollback, dev vs production
// pages), App messages, Datastar redirects as navigation events; and
// TestE2E_RequiresUnderDatastar.
func TestE2E_Todos(t *testing.T) {
	runE2E(t, "../../examples/todos", "testdata/e2e_todos_test.go.txt", "TestE2E_Todos", "TestE2E_RequiresUnderDatastar")
}

// TestE2E_RejectKeepsWrites runs gen_vet/m3c-reject (an update that inserts a row, stores the
// session and then Rejects) through Serve: the row and the session are kept, the form shows the
// rejection (testdata/e2e_reject_test.go.txt).
func TestE2E_RejectKeepsWrites(t *testing.T) {
	runE2E(t, "../../testdata/golden/gen_vet/m3c-reject", "testdata/e2e_reject_test.go.txt", "TestE2E_RejectKeepsWrites")
}

// runE2E emits app, adds program as zz_e2e_test.go and runs the tests named, each of which
// must pass (a -run pattern matching nothing would pass silently).
func runE2E(t *testing.T, app, program string, tests ...string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	src, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	dir := emitApp(t, app)
	if err := os.WriteFile(filepath.Join(dir, "zz_e2e_test.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-count=1", "-v", "-run", "^("+strings.Join(tests, "|")+")$", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test in the emitted app failed: %v\n%s", err, out)
	}
	for _, name := range tests {
		if !regexp.MustCompile(`(?m)^--- PASS: ` + name + ` `).Match(out) {
			t.Fatalf("%s did not pass:\n%s", name, out)
		}
	}
	if testing.Verbose() {
		t.Logf("%s", out)
	}
}
