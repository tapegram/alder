package gen

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An App write route in an app with silos (the `new msg App` shape, 3665907b): App is no
// route module with an `org` in scope, so its update can only write the shared store (E0026 refuses
// a silo table there, see the E0026-silo-unscoped-outside corpus). The generated route still
// enters a store, the shared one (`db.Enter(rq.Env, "")`, routes.go), and the write lands in the
// shared database, not in a silo's. The app is silo-routes-ok with `route Note` added to App.
func TestGen_AppWriteRouteEntersSilo(t *testing.T) {
	src := "../../testdata/corpus/silo-routes-ok/app"
	dir := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "App.alder" {
			b = append(b, []byte(`
route Note = POST "/_/Note" { name: Text }

Note.update : Note.Params -> Session -> Task Response
Note.update params session = {
  let _ <- Db.insert Org { name: params.name }
  Task.succeed Response.refresh
}
`)...)
		}
		out := filepath.Join(dir, "app", rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	files := emit(t, dir)
	routes := string(files["routes__alder.go"])
	i := strings.Index(routes, "func Route__App__Note() web.Route {")
	if i < 0 {
		t.Fatalf("no Route__App__Note in:\n%s", routes)
	}
	body := routes[i:]
	if j := strings.Index(body[1:], "\nfunc "); j >= 0 {
		body = body[:j+1]
	}
	if !strings.Contains(body, `db.Enter(rq.Env, "")`) {
		t.Errorf("App's write route does not enter the shared store:\n%s", body)
	}
	runGo(t, files, `package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/web"
)

func TestAppWriteRouteSilo(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	path := filepath.Join(t.TempDir(), "app.db")
	h, err := Serve(Caps{}, Options{DB: path, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	c, err := web.SessionCookie(secret, session__codec{}, App__Session{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/_/Note?name=acme", strings.NewReader(url.Values{}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Cookie", web.CookieName+"="+c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code >= 500 || w.Code == http.StatusNotFound {
		t.Fatalf("POST /_/Note: %d %q", w.Code, w.Body.String())
	}
	store, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	n, err := store.Count(context.Background(), nil, "Org.Org", db.SQL{})
	if err != nil || n != 1 {
		t.Fatalf("shared Org rows = %d, %v (POST: %d %q)", n, err, w.Code, w.Body.String())
	}
	if es, _ := os.ReadDir(filepath.Join(filepath.Dir(path), "silos")); len(es) != 0 {
		t.Errorf("silo databases were opened: %v", es)
	}
}
`)
}
