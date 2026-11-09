package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runRoutes(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runRoutesTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRoutes_Table(t *testing.T) {
	t.Parallel()
	code, out, errs := runRoutes("../../examples/blog")
	if code != 0 || errs != "" {
		t.Fatalf("code %d, err %s", code, errs)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 || strings.Fields(lines[0])[0] != "METHOD" {
		t.Fatalf("no header:\n%s", out)
	}
	for _, w := range []string{"GET     /posts/{slug}", "Posts.Show.Show", "app/Posts/Show.alder:9", "POST    /posts/{slug}/_/AddComment", "Posts.Show.AddComment", "app/Posts/Show.alder:12"} {
		if !strings.Contains(out, w) {
			t.Errorf("table lacks %q:\n%s", w, out)
		}
	}
}

func TestRoutes_JSON(t *testing.T) {
	t.Parallel()
	code, out, errs := runRoutes("--json", "../../examples/blog")
	if code != 0 || errs != "" {
		t.Fatalf("code %d, err %s", code, errs)
	}
	var rs []struct {
		Kind, Method, Path, Module, Name, File string
		Line, Col                              int
	}
	if err := json.Unmarshal([]byte(out), &rs); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var page, msg bool
	for _, r := range rs {
		if r.Kind == "route" && r.Method == "GET" && r.Path == "/posts/{slug}" && r.Module == "Posts.Show" && r.File == "app/Posts/Show.alder" && r.Line == 9 && r.Col == 1 {
			page = true
		}
		if r.Kind == "route" && r.Method == "POST" && r.Module == "Posts.Show" && r.Name == "AddComment" && r.File == "app/Posts/Show.alder" && r.Line == 12 {
			msg = true
		}
	}
	if !page || !msg {
		t.Fatalf("page %v msg %v:\n%s", page, msg, out)
	}
}

func TestRoutes_Errors(t *testing.T) {
	t.Parallel()
	code, _, _ := runRoutes(t.TempDir())
	if code != 2 {
		t.Errorf("empty dir: code %d", code)
	}
	code, out, errs := runRoutes("../../testdata/corpus/E0002-own-module")
	if code != 1 || out != "" || !strings.Contains(errs, "E0002") {
		t.Errorf("type error: code %d, out %q, err %q", code, out, errs)
	}
}

func TestRoutes_RouteApp(t *testing.T) {
	t.Parallel()
	code, out, errs := runRoutes("--json", "../../testdata/corpus/routes-ok")
	if code != 0 || errs != "" {
		t.Fatalf("code %d, err %s", code, errs)
	}
	var rs []struct {
		Method, Kind, Path, Module, Name string
	}
	if err := json.Unmarshal([]byte(out), &rs); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var del bool
	for _, r := range rs {
		if r.Method == "DELETE" && r.Kind == "route" && r.Path == "/todos/{id}" && r.Name == "Delete" {
			del = true
		}
	}
	if !del {
		t.Errorf("no DELETE Delete:\n%s", out)
	}
	code, tab, _ := runRoutes("../../testdata/corpus/routes-ok")
	if code != 0 || !strings.Contains(tab, "METHOD") || !strings.Contains(tab, "DELETE") || !strings.Contains(tab, "Todos.Delete") {
		t.Errorf("table:\n%s", tab)
	}
}

// A built app renders the route trigger attributes (#160): load, every, intersect and input,
// on a plain control and on <Input>; the `every` attribute itself is not rendered.
func TestRoutes_TriggerAttrsRender(t *testing.T) {
	r := startRunApp(t, "../testdata/corpus/routes-attrs-ok")
	body := get(t, r.base+"/")
	for _, want := range []string{
		`data-init="@get(&#39;/stats`,
		`data-on-interval__duration.60s=`,
		`data-on-interval__duration.5s=`,
		`data-on-interval__duration.500ms=`,
		`data-on-intersect__once=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, body)
		}
	}
	if n := strings.Count(body, `data-on:input__debounce.300ms=`); n != 2 {
		t.Errorf("%d input triggers, want 2 (plain and <Input>):\n%s", n, body)
	}
	if strings.Contains(body, "every=") {
		t.Errorf("page renders an every attribute:\n%s", body)
	}
}
