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
	formless := &Route{Module: "Replies.Show", Name: "Delete", Method: "DELETE"}
	withForm := &Route{Module: "Replies.Show", Name: "Delete", Method: "DELETE", Form: "Gone"}
	for _, c := range []struct {
		name string
		have *Route
		from string
		form bool
		want string
	}{
		{"form, same module, no form yet", formless, "Replies.Show", true,
			"routes `Replies.Show.Delete` and `Remove` both answer DELETE /replies/{id}; give the new route its own resource with `--path`, or attach the form to `Replies.Show.Delete` with `--msg Delete`"},
		{"form, same module, has a form", withForm, "Replies.Show", true,
			"routes `Replies.Show.Delete` and `Remove` both answer DELETE /replies/{id}; give the new route its own resource with `--path`"},
		{"msg, same module", formless, "Replies.Show", false,
			"routes `Replies.Show.Delete` and `Remove` both answer DELETE /replies/{id}; give the new route its own resource with `--path` (and `--method` for what happens to it)"},
		{"form, other module", formless, "Home", true,
			"routes `Replies.Show.Delete` and `Remove` both answer DELETE /replies/{id}; give the new route its own resource with `--path`; or, to send `Replies.Show.Delete` itself, write it into `Home`'s `view` by hand: it is a route of `Replies.Show`"},
	} {
		err := clashRefusal(c.have, c.from, "Remove", "DELETE", "/replies/{id}", c.form)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: got %v\nwant %s", c.name, err, c.want)
		}
	}
}

// TestParseAddress_LegacyUpdate: `Module.Route.update` and bare `Module.update` name the handler
// of an app whose route still has `update` (W0013), and of one whose handler is named after its
// method; a route with neither is refused without a panic.
func TestParseAddress_LegacyUpdate(t *testing.T) {
	p := clashApp(t)
	for _, addr := range []string{"Replies.Show.Delete.update", "Replies.Show.update", "Replies.Show.Delete.delete"} {
		mod, decl, _, err := parseAddress(p, addr)
		if addr == "Replies.Show.Delete.delete" {
			if err != nil || mod != "Replies.Show" || decl != "Delete.delete" {
				t.Errorf("%s: %q %q %v", addr, mod, decl, err)
			}
			continue
		}
		if err != nil || mod != "Replies.Show" || decl != "Delete.update" {
			t.Errorf("%s: %q %q %v", addr, mod, decl, err)
		}
	}
	if got := p.handlerIn("Replies.Show", "Delete", "DELETE"); got != "update" {
		t.Errorf("handlerIn legacy = %q", got)
	}
	if got := p.handlerIn("Nope", "Delete", "DELETE"); got != "delete" {
		t.Errorf("handlerIn new = %q", got)
	}
}
