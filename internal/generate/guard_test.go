package generate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/syntax"
)

// named is a Guard: the value declaration called name.
func named(name string) func(syntax.Decl) bool {
	return func(d syntax.Decl) bool { v, ok := d.(*syntax.ValueDecl); return ok && v.Name == name }
}

const guardNote = "module Note\n\na : Int\na = 1\n\nb : Int\nb = 2\n"

// refused asserts Apply refused (a *Refusal whose message contains want) and app/Note.alder is
// still guardNote.
func refused(t *testing.T, dir string, err error, want string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(r.Msg, want) {
		t.Fatalf("err = %v, want a refusal containing %q", err, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Note.alder")); string(b) != guardNote {
		t.Errorf("written despite the refusal:\n%s", b)
	}
}

// TestGuard_DeclarationMissingRefuses: a Guard whose declaration is not in the file after the
// edit refuses it, even under WriteDespiteErrors.
func TestGuard_DeclarationMissingRefuses(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Note.alder": guardNote})
	_, err := Apply(dir, []Edit{{Path: "app/Note.alder", Body: guardNote + "\nc : Int\nc = 3\n", Guard: named("missing")}}, Options{WriteDespiteErrors: true})
	refused(t, dir, err, "the declaration the command edits was not found after the edit")
}

// TestSplice_BreaksParseRefuses: an edited file that parsed before the edit and does not after
// it is refused, even under WriteDespiteErrors (a parse error after a splice is the generator's).
func TestSplice_BreaksParseRefuses(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Note.alder": guardNote})
	_, err := Apply(dir, []Edit{{Path: "app/Note.alder", Body: guardNote + "\nc : Int\nc = (\n"}}, Options{WriteDespiteErrors: true})
	refused(t, dir, err, "`app/Note.alder` does not parse after the edit")
	var r *Refusal
	if errors.As(err, &r) && (len(r.Diags) == 0 || r.Sources["app/Note.alder"] == nil) {
		t.Errorf("the refusal does not carry the parse errors and source: %+v", r)
	}
}

// TestGuard_TwoOnOnePath: two guarded edits of one file each guard their own declaration; the
// second does not replace the first.
func TestGuard_TwoOnOnePath(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Note.alder": guardNote})
	body := strings.Replace(guardNote, "a = 1", "a = \"one\"", 1) // an error in a, none in b
	_, err := Apply(dir, []Edit{
		{Path: "app/Note.alder", Body: body, Guard: named("a")},
		{Path: "app/Note.alder", Body: body, Guard: named("b")},
	}, Options{WriteDespiteErrors: true})
	refused(t, dir, err, "the declaration the command edits does not check")
}

// TestGuard_ErrorElsewhereWrites: under WriteDespiteErrors an error outside the guarded
// declaration is the caller's to-do list: the edit is written.
func TestGuard_ErrorElsewhereWrites(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Note.alder": guardNote})
	body := strings.Replace(guardNote, "a = 1", "a = \"one\"", 1)
	res, err := Apply(dir, []Edit{{Path: "app/Note.alder", Body: body, Guard: named("b")}}, Options{WriteDespiteErrors: true})
	if err != nil || !res.Written {
		t.Fatalf("err = %v, res = %+v", err, res)
	}
}
