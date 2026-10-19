package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApply_Texts(t *testing.T) {
	dir := scaffoldApp(t, nil)
	home, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	edits := []Edit{
		{Path: "app/Home.alder", Body: string(home) + "\n-- edited\n"},
		{Path: "app/Extra.alder", Create: true, Body: "module Extra\n"},
	}
	res, err := Apply(dir, edits, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]FileChange{}
	for _, f := range res.Files {
		got[f.Path] = f
	}
	h := got["app/Home.alder"]
	if h.Before == nil || *h.Before != string(home) || h.After == nil || !strings.HasSuffix(*h.After, "-- edited\n") {
		t.Fatalf("edited texts: %+v", h)
	}
	x := got["app/Extra.alder"]
	if x.Before != nil || x.After == nil || *x.After != "module Extra\n" {
		t.Fatalf("created texts: %+v", x)
	}
}

func TestMaterialise_Overlay(t *testing.T) {
	dir := scaffoldApp(t, nil)
	tmp, cleanup, err := Materialise(dir, map[string][]byte{"app/Home.alder": []byte("module Home\n-- unsaved\n")})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	b, _ := os.ReadFile(filepath.Join(tmp, "app", "Home.alder"))
	if string(b) != "module Home\n-- unsaved\n" {
		t.Fatalf("overlay not written: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder")); strings.Contains(string(b), "unsaved") {
		t.Fatal("the real app was changed")
	}
	cleanup()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("cleanup left the copy")
	}
}
