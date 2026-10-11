package buildkit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/ir"
)

// The rule: capability X of module A.B.… is func B…X of package adapters/a.
func TestAdapterRule(t *testing.T) {
	for _, c := range []struct{ module, cap, dir, fn, iface string }{
		{"Auth.Login", "Password", "auth", "LoginPassword", "app.Cap__Auth__Login__Password"},
		{"Auth.Signup", "Password", "auth", "SignupPassword", "app.Cap__Auth__Signup__Password"},
		{"Markdown", "Markdown", "markdown", "Markdown", "app.Cap__Markdown__Markdown"},
		{"Payments", "Stripe", "payments", "Stripe", "app.Cap__Payments__Stripe"},
		{"App", "Clock", "app", "Clock", "app.Cap__App__Clock"},
		{"Billing.Plans.Annual", "Invoice", "billing", "PlansAnnualInvoice", "app.Cap__Billing__Plans__Annual__Invoice"},
	} {
		if got := AdapterPackage(c.module); got != c.dir {
			t.Errorf("AdapterPackage(%q) = %q, want %q", c.module, got, c.dir)
		}
		if got := Constructor(c.module, c.cap); got != c.fn {
			t.Errorf("Constructor(%q, %q) = %q, want %q", c.module, c.cap, got, c.fn)
		}
		n := Needs([]*ir.Capability{{Module: c.module, Name: c.cap}})[0]
		if want := (Need{c.module, c.cap, c.dir, c.fn, c.iface}); n != want {
			t.Errorf("Needs: %+v, want %+v", n, want)
		}
	}
	n := Needs([]*ir.Capability{{Module: "Auth.Login", Name: "Password"}})[0]
	if got, want := n.String(), "capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`"; got != want {
		t.Errorf("Need.String() = %q, want %q", got, want)
	}
}

func caps(names ...string) []*ir.Capability {
	var out []*ir.Capability
	for _, n := range names {
		i := strings.LastIndex(n, ".")
		out = append(out, &ir.Capability{Module: n[:i], Name: n[i+1:]})
	}
	return out
}

func TestScanAdapters(t *testing.T) {
	all := Needs(caps("Auth.Login.Password", "Auth.Signup.Password", "Markdown.Markdown", "Payments.Stripe"))
	for _, c := range []struct {
		fixture string
		missing []string // Module.Cap
		flat    bool
	}{
		// a test file's function and a helper package are no constructors; Payments has no package
		{"ok", []string{"Payments.Stripe"}, false},
		// unexported, a method, excluded by a build constraint, taking a parameter, no .go file
		{"missing", []string{"Auth.Login.Password", "Auth.Signup.Password", "Markdown.Markdown", "Payments.Stripe"}, false},
		{"flat", []string{"Auth.Login.Password", "Auth.Signup.Password", "Markdown.Markdown", "Payments.Stripe"}, true},
		{"absent", []string{"Auth.Login.Password", "Auth.Signup.Password", "Markdown.Markdown", "Payments.Stripe"}, false},
	} {
		missing, flat, err := ScanAdapters(filepath.Join("testdata", "adapters", c.fixture), all)
		if err != nil {
			t.Fatalf("%s: %v", c.fixture, err)
		}
		var got []string
		for _, n := range missing {
			got = append(got, n.Module+"."+n.Cap)
		}
		if !slices.Equal(got, c.missing) || flat != c.flat {
			t.Errorf("%s: missing %v, flat %v; want %v, %v", c.fixture, got, flat, c.missing, c.flat)
		}
	}
}

// A package with a file that does not parse is taken to have every constructor: Go reports
// the syntax error.
func TestScanAdapters_ParseError(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "adapters", "auth", "a.go"), "package auth\n\nfunc LoginPassword( {\n")
	missing, _, err := ScanAdapters(dir, Needs(caps("Auth.Login.Password")))
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing %v, err %v", missing, err)
	}
}

func TestCheckAdapters(t *testing.T) {
	for _, c := range []struct {
		fixture string
		caps    []*ir.Capability
		ok      bool
		want    string
	}{
		{"ok", caps("Auth.Login.Password", "Markdown.Markdown"), true, ""},
		{"ok", nil, true, ""},
		{"ok", caps("Payments.Stripe"), false, "ok is missing adapters for 1 capability:\n" +
			"capability `Payments.Stripe` needs `adapters/payments/` with `func Stripe() app.Cap__Payments__Stripe`\n"},
		{"missing", caps("Auth.Login.Password", "Markdown.Markdown"), false, "missing is missing adapters for 2 capabilities:\n" +
			"capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`\n" +
			"capability `Markdown.Markdown` needs `adapters/markdown/` with `func Markdown() app.Cap__Markdown__Markdown`\n"},
		{"flat", caps("Auth.Login.Password"), false, "adapters/ no longer wires capabilities: move each adapter to adapters/<module>/ (see the list below)\n" +
			"flat is missing adapters for 1 capability:\n" +
			"capability `Auth.Login.Password` needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`\n"},
		{"flat", nil, false, "adapters/ no longer wires capabilities: delete func Caps (adapters/caps.go): the generated caps package wires the adapters\n"},
		{"ok", caps("Internal.Mail.Smtp", "Internal.Log", "Testdata.Seed", "Auth.Login.Password"), false,
			"module `Internal` cannot name an adapter package: Go keeps `adapters/internal/` private to adapters/ (it holds helpers); rename the module\n" +
				"module `Testdata` cannot name an adapter package: Go skips `testdata/` directories; rename the module\n"},
		{"ok", caps("A.BC.D", "A.B.CD"), false, "internal naming error: capabilities A.BC.D and A.B.CD both need func BCD in adapters/a/ (rename one of them)\n"},
	} {
		var out strings.Builder
		dir := filepath.Join("testdata", "adapters", c.fixture)
		if ok := CheckAdapters(dir, c.caps, &out); ok != c.ok || out.String() != c.want {
			t.Errorf("%s %v: ok %v, printed:\n%s\nwant %v:\n%s", c.fixture, c.caps, ok, out.String(), c.ok, c.want)
		}
	}
}

func TestHasAdapters(t *testing.T) {
	dir := t.TempDir()
	if HasAdapters(dir) {
		t.Error("no adapters/: HasAdapters")
	}
	write(t, filepath.Join(dir, "adapters", "README.md"), "notes")
	if HasAdapters(dir) {
		t.Error("adapters/ of notes only: HasAdapters")
	}
	write(t, filepath.Join(dir, "adapters", "auth", "password.go"), "package auth\n")
	if !HasAdapters(dir) {
		t.Error("adapters/auth/password.go: !HasAdapters")
	}
}

// CopyAdapters copies the whole tree (subdirectories, test files, testdata/) and removes what
// the source no longer has, leaving unchanged files alone.
func TestCopyAdapters(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "adapters")
	files := map[string]string{
		"auth/password.go":           "package auth\n",
		"auth/password_test.go":      "package auth\n",
		"auth/testdata/users.txt":    "ada\n",
		"internal/dbx/dbx.go":        "package dbx\n",
		"markdown/markdown.go":       "package markdown\n",
		"markdown/templates/a.tmpl":  "{{.}}\n",
		"stale/old.go":               "package stale\n",
		"auth/stale.go":              "package auth\n",
		"auth/testdata/old/users.go": "package old\n",
	}
	for name, body := range files {
		write(t, filepath.Join(src, name), body)
	}
	if err := CopyAdapters(src, dst, true); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, dst); !slices.Equal(got, sortedKeys(files)) {
		t.Fatalf("copied %v, want %v", got, sortedKeys(files))
	}
	for _, gone := range []string{"stale", "auth/stale.go", "auth/testdata/old"} {
		if err := os.RemoveAll(filepath.Join(src, gone)); err != nil {
			t.Fatal(err)
		}
		for name := range files {
			if name == gone || strings.HasPrefix(name, gone+"/") {
				delete(files, name)
			}
		}
	}
	files["auth/password.go"] = "package auth\n\nfunc LoginPassword() {}\n"
	write(t, filepath.Join(src, "auth/password.go"), files["auth/password.go"])
	if err := CopyAdapters(src, dst, true); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, dst); !slices.Equal(got, sortedKeys(files)) {
		t.Fatalf("after removals: %v, want %v", got, sortedKeys(files))
	}
	for name, body := range files {
		if b, _ := os.ReadFile(filepath.Join(dst, name)); string(b) != body {
			t.Errorf("%s = %q, want %q", name, b, body)
		}
	}
	for _, d := range []string{"stale", "auth/testdata/old"} {
		if _, err := os.Stat(filepath.Join(dst, d)); !os.IsNotExist(err) {
			t.Errorf("%s still exists (%v)", d, err)
		}
	}
	if err := CopyAdapters(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("not present: %s still exists (%v)", dst, err)
	}
}

// WriteGenerated writes caps/caps__alder.go into its directory and removes it (and the
// directory) when a later build has no capabilities.
func TestWriteGenerated_Caps(t *testing.T) {
	dir := t.TempDir()
	caps := []byte(GeneratedHeader + "; DO NOT EDIT.\n\npackage caps\n")
	if err := WriteGenerated(dir, map[string][]byte{"go.mod": []byte("module app\n"), "caps/caps__alder.go": caps}); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "caps", "caps__alder.go")); err != nil || string(b) != string(caps) {
		t.Fatalf("caps file: %q, %v", b, err)
	}
	if err := WriteGenerated(dir, map[string][]byte{"go.mod": []byte("module app\n")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "caps")); !os.IsNotExist(err) {
		t.Errorf("caps/ still exists (%v)", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tree lists the files under dir as sorted slash paths.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
