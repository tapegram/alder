package generate

import (
	"os"
	"path/filepath"
	"testing"
)

// clashApp is the fixture plus module Replies.Show, which has `route Delete = DELETE
// "/replies/{id}"`.
func clashApp(t *testing.T) *Program {
	t.Helper()
	dir := t.TempDir()
	if err := copyApp(fixture, dir, nil); err != nil {
		t.Fatal(err)
	}
	src := "module Replies.Show\n\nroute Delete = DELETE \"/replies/{id}\" { id: Int }\n\nDelete.update : Delete.Params -> Session -> Task Response\nDelete.update p session = Task.succeed Response.refresh\n"
	f := filepath.Join(dir, "app", "Replies", "Show.alder")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRouteClash_Found(t *testing.T) {
	r := routeClash(clashApp(t), "DELETE", "/replies/{id}", "")
	if r == nil || r.Module != "Replies.Show" || r.Name != "Delete" {
		t.Errorf("got %+v", r)
	}
}

func TestRouteClash_ParamNamesIgnored(t *testing.T) {
	if routeClash(clashApp(t), "DELETE", "/replies/{rid}", "") == nil {
		t.Error("want a clash")
	}
}

func TestRouteClash_MethodDiffers(t *testing.T) {
	if r := routeClash(clashApp(t), "PUT", "/replies/{id}", ""); r != nil {
		t.Errorf("got %+v", r)
	}
}

func TestRouteClash_TrailingSlash(t *testing.T) {
	if routeClash(clashApp(t), "DELETE", "/replies/{id}/", "") == nil {
		t.Error("want a clash")
	}
}

func TestRouteClash_Except(t *testing.T) {
	if r := routeClash(clashApp(t), "DELETE", "/replies/{id}", "Replies.Show.Delete"); r != nil {
		t.Errorf("got %+v", r)
	}
}

func TestClashRefusal_Text(t *testing.T) {
	have := &Route{Module: "Replies.Show", Name: "Delete"}
	err := clashRefusal(have, "Remove", "DELETE", "/replies/{id}")
	want := "routes `Replies.Show.Delete` and `Remove` both answer DELETE /replies/{id}; give the new route its own resource with `--path`, or attach the form to `Replies.Show.Delete` with `--msg Delete`"
	if err == nil || err.Error() != want {
		t.Errorf("got %v\nwant %s", err, want)
	}
}
