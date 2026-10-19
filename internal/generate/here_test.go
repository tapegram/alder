package generate

import (
	"testing"
)

func TestHere_PerSpec(t *testing.T) {
	page := HereCtx{Module: "Posts.Index", IsPage: true, Decl: "view", DeclKind: "value"}
	table := HereCtx{Module: "Post", Decl: "Post", DeclKind: "table"}
	plain := HereCtx{Module: "Markdown"}
	cases := []struct {
		ctx  HereCtx
		id   string
		want map[string]string // nil: not here
	}{
		{page, "new msg", map[string]string{"page": "Posts.Index"}},
		{page, "new form", map[string]string{"page": "Posts.Index"}},
		{page, "edit page", map[string]string{"page": "Posts.Index"}},
		{page, "remove page", map[string]string{"page": "Posts.Index"}},
		{page, "new test", map[string]string{"module": "Posts.Index"}},
		{page, "edit decl", map[string]string{"address": "Posts.Index.view"}},
		{table, "edit table", map[string]string{"table": "Post"}},
		{table, "remove table", map[string]string{"table": "Post"}},
		{table, "new msg", nil},
		{plain, "new decl", map[string]string{"module": "Markdown"}},
		{plain, "edit decl", nil},
		{plain, "new page", nil},
	}
	for _, c := range cases {
		s := SpecByID(c.id)
		var m *Match
		if s.Here != nil {
			m = s.Here(c.ctx)
		}
		if c.want == nil {
			if m != nil {
				t.Errorf("%s in %+v: here %v, want none", c.id, c.ctx, m)
			}
			continue
		}
		if m == nil || m.Why == "" || len(m.Sets) != len(c.want) {
			t.Errorf("%s in %+v: %v, want %v", c.id, c.ctx, m, c.want)
			continue
		}
		for k, v := range c.want {
			if m.Sets[k] != v {
				t.Errorf("%s: %s = %q, want %q", c.id, k, m.Sets[k], v)
			}
		}
	}
}

func TestForm_Validation(t *testing.T) {
	dir := scaffoldApp(t, nil)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := SpecByID("new msg")
	fields, _, ready := Form(s, &Match{Sets: map[string]string{"page": "Home"}, Why: "x"}, nil, p)
	if ready {
		t.Fatal("ready without a name")
	}
	byName := map[string]FieldState{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if f := byName["page"]; !f.Valid || f.Source != "here" || len(f.Choices) == 0 {
		t.Fatalf("page: %+v", f)
	}
	if f := byName["name"]; f.Valid || f.Why != "required" {
		t.Fatalf("name: %+v", f)
	}
	fields, sets, ready := Form(s, &Match{Sets: map[string]string{"page": "Home"}}, map[string]string{"name": "Ping", "fields": "n:Int  label:Text"}, p)
	if !ready || sets.One("name") != "Ping" || len(sets.All("fields")) != 2 {
		t.Fatalf("ready %v sets %v fields %+v", ready, sets, fields)
	}
	_, _, ready = Form(s, nil, map[string]string{"page": "Nope", "name": "Ping"}, p)
	if ready {
		t.Fatal("ready with an unknown page")
	}
}

func TestForm_NoProgram(t *testing.T) {
	fields, _, ready := Form(SpecByID("new page"), nil, map[string]string{"module": "posts", "path": "x"}, nil)
	if ready {
		t.Fatal("ready with a bad module and route")
	}
	if fields[0].Why == "" || fields[1].Why != "a path starts with /" {
		t.Fatalf("whys: %+v", fields[:2])
	}
}

func TestForm_Default(t *testing.T) {
	dir := scaffoldApp(t, nil)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := &Spec{ID: "x y", Args: []Field{{Name: "name", Kind: ArgText, Required: true}},
		Flags: []Field{{Name: "example", Kind: ArgBool, Default: "true"}, {Name: "strict", Kind: ArgBool}}}
	fields, sets, _ := Form(s, nil, nil, p)
	byName := map[string]FieldState{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	if f := byName["example"]; f.Value != "true" || f.Source != "default" || !f.Valid {
		t.Fatalf("example: %+v", f)
	}
	if f := byName["strict"]; f.Value != "" || f.Source != "" {
		t.Fatalf("strict has no default: %+v", f)
	}
	if !sets.Has("example") || sets.Has("strict") {
		t.Fatalf("sets %v", sets)
	}
	// you win over the default
	_, sets, _ = Form(s, nil, map[string]string{"example": "false", "name": "A"}, p)
	if sets.Has("example") {
		t.Fatalf("you = false still set: %v", sets)
	}
	// here wins over the default too
	_, sets, _ = Form(s, &Match{Sets: map[string]string{"example": "false"}}, nil, p)
	if sets.Has("example") {
		t.Fatalf("here = false still set: %v", sets)
	}
}
