package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// /_dev/map.json is the dev map as JSON: the generator's map with the new SQL names and silos
// passing through, the runtime's build id, capabilities, migrations and routes beside it; only
// under ALDER_DEV=1.
func TestRouter_MapJSON(t *testing.T) {
	route := one("GET", "/x/{id}", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil) })
	opts := func(o *Options) {
		o.Dev = true
		o.Migrations = []string{"0001_init"}
		o.DevMap = `{"routes":[{"method":"GET","module":"X","path":"/x/{id}"}],` +
			`"tables":[{"name":"Shelf.Book","sql":"Shelf__Book","columns":["title: Text"],"cols":[{"name":"title","type":"Text","sql":"title","sqlType":"TEXT"}],"silo":"org"}],` +
			`"silos":[{"key":"org","ref":"Org.Org","tables":["Shelf.Book"],"path":"silos/org/{value}.db"}],"caps":[],"session":[]}`
	}

	t.Run("dev", func(t *testing.T) {
		t.Setenv("ALDER_DEV", "1")
		w := serve(NewRouter(route, testOpts(opts)), httptest.NewRequest("GET", "/_dev/map.json", nil))
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
		}
		var got struct {
			Build string
			Map   struct {
				Routes []struct{ Method, Module, Path string }
				Tables []struct {
					Name, SQL, Silo string
					Cols            []struct{ Name, Type, SQL, SQLType string }
				}
				Silos []struct {
					Key, Ref, Path string
					Tables         []string
				}
			}
			Caps       []struct{ Name, Kind string }
			Migrations []string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		tb := got.Map.Tables
		if got.Build == "" || len(tb) != 1 || tb[0].SQL != "Shelf__Book" || tb[0].Silo != "org" || len(tb[0].Cols) != 1 || tb[0].Cols[0].SQLType != "TEXT" {
			t.Errorf("map = %s", w.Body.String())
		}
		if s := got.Map.Silos; len(s) != 1 || s[0].Key != "org" || s[0].Path != "silos/org/{value}.db" || s[0].Tables[0] != "Shelf.Book" {
			t.Errorf("silos = %+v", s)
		}
		if len(got.Migrations) != 1 || got.Migrations[0] != "0001_init" {
			t.Errorf("migrations = %v", got.Migrations)
		}
		if r := got.Map.Routes; len(r) != 1 || r[0].Method != "GET" || r[0].Module != "X" || r[0].Path != "/x/{id}" {
			t.Errorf("routes = %+v", r)
		}
		page := serve(NewRouter(route, testOpts(opts)), httptest.NewRequest("GET", "/_dev", nil)).Body.String()
		if n := strings.Count(page, "/x/{id}"); n != 1 {
			t.Errorf("/_dev lists /x/{id} %d times, want once: %s", n, page)
		}
	})

	t.Run("prod", func(t *testing.T) {
		t.Setenv("ALDER_DEV", "")
		w := serve(NewRouter(route, testOpts(func(o *Options) { opts(o); o.Dev = false })), httptest.NewRequest("GET", "/_dev/map.json", nil))
		if w.Code != 404 {
			t.Fatalf("%d %q", w.Code, w.Body.String())
		}
	})
}
