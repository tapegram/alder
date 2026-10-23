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
	code, out, errs := runRoutes("../../examples/blog")
	if code != 0 || errs != "" {
		t.Fatalf("code %d, err %s", code, errs)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 || strings.Fields(lines[0])[0] != "KIND" {
		t.Fatalf("no header:\n%s", out)
	}
	for _, w := range []string{"page  /posts/{slug}", "Posts.Show.page", "app/Posts/Show.alder:9", "msg   /_m/Posts.Show/AddComment", "Posts.Show.AddComment", "app/Posts/Show.alder:44"} {
		if !strings.Contains(out, w) {
			t.Errorf("table lacks %q:\n%s", w, out)
		}
	}
}

func TestRoutes_JSON(t *testing.T) {
	code, out, errs := runRoutes("--json", "../../examples/blog")
	if code != 0 || errs != "" {
		t.Fatalf("code %d, err %s", code, errs)
	}
	var rs []struct {
		Kind, Path, Module, Name, File string
		Line, Col                      int
	}
	if err := json.Unmarshal([]byte(out), &rs); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var page, msg bool
	for _, r := range rs {
		if r.Kind == "page" && r.Path == "/posts/{slug}" && r.Module == "Posts.Show" && r.File == "app/Posts/Show.alder" && r.Line == 9 && r.Col == 1 {
			page = true
		}
		if r.Kind == "msg" && r.Module == "Posts.Show" && r.Name == "AddComment" && r.File == "app/Posts/Show.alder" && r.Line == 44 {
			msg = true
		}
	}
	if !page || !msg {
		t.Fatalf("page %v msg %v:\n%s", page, msg, out)
	}
}

func TestRoutes_Errors(t *testing.T) {
	code, _, _ := runRoutes(t.TempDir())
	if code != 2 {
		t.Errorf("empty dir: code %d", code)
	}
	code, out, errs := runRoutes("../../testdata/corpus/E0002-own-module")
	if code != 1 || out != "" || !strings.Contains(errs, "E0002") {
		t.Errorf("type error: code %d, out %q, err %q", code, out, errs)
	}
}
