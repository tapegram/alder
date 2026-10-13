package generate

import (
	"slices"
	"strings"
	"testing"
)

const regionSrc = `module Admin.Post.Index

-- the author's comment
title : Text
title = "Posts"

  -- @admin begin columns
  a
  b
  -- @admin end

x = 1
-- @admin begin empty
-- @admin end
`

func TestRegions_Replace(t *testing.T) {
	got, err := Regions.Replace(regionSrc, "columns", "  c\n\n  d")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(regionSrc, "  a\n  b\n", "  c\n\n  d\n", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// an empty body leaves the markers alone; a body fills an empty region
	if got, _ := Regions.Replace(regionSrc, "columns", ""); got != strings.Replace(regionSrc, "  a\n  b\n", "", 1) {
		t.Errorf("emptied:\n%s", got)
	}
	if got, _ := Regions.Replace(regionSrc, "empty", "y = 2"); got != strings.Replace(regionSrc, "begin empty\n", "begin empty\ny = 2\n", 1) {
		t.Errorf("filled:\n%s", got)
	}
	// several at once, the text outside untouched
	got, err = ReplaceRegions(regionSrc, []Region{{"empty", "z"}, {"columns", "  q"}})
	if err != nil || !strings.Contains(got, "-- the author's comment\ntitle : Text\ntitle = \"Posts\"\n\n  -- @admin begin columns\n  q\n  -- @admin end\n\nx = 1\n-- @admin begin empty\nz\n-- @admin end\n") {
		t.Errorf("both: %v\n%s", err, got)
	}
}

func TestRegions_Idempotent(t *testing.T) {
	body, ok, err := Regions.Body(regionSrc, "columns")
	if err != nil || !ok || body != "  a\n  b" {
		t.Fatalf("Body: %q %v %v", body, ok, err)
	}
	once, _ := Regions.Replace(regionSrc, "columns", body)
	if once != regionSrc {
		t.Errorf("replacing a region with its own body changed the file:\n%s", once)
	}
	twice, _ := Regions.Replace(once, "columns", body)
	if twice != once {
		t.Errorf("not idempotent")
	}
	if names, err := Regions.Names(regionSrc); err != nil || !slices.Equal(names, []string{"columns", "empty"}) {
		t.Errorf("Names: %v %v", names, err)
	}
}

func TestRegions_Broken(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"-- @admin begin columns\na\n", "region `columns` has no end marker"},
		{"a\n-- @admin end\n", "the end marker at line 2 has no begin marker"},
		{"-- @admin begin a\n-- @admin begin b\n-- @admin end\n-- @admin end\n", "region `a` has no end marker (region `b` begins at line 2, inside it)"},
		{"-- @admin begin a\n-- @admin end\n-- @admin begin a\n-- @admin end\n", "region `a` begins twice, at lines 1 and 3"},
		{"  -- @admin begin\n-- @admin end\n", "the begin marker at line 1 has no region name: write `-- @admin begin <name>`"},
	} {
		err := Regions.Check(c.src)
		if err == nil || err.Error() != c.want {
			t.Errorf("Check(%q) = %v, want %q", c.src, err, c.want)
		}
		if _, err := Regions.Replace(c.src, "a", "x"); err == nil {
			t.Errorf("Replace on a broken file did not refuse: %q", c.src)
		}
	}
	if _, err := Regions.Replace(regionSrc, "nope", "x"); err == nil || err.Error() != "there is no region `nope`" {
		t.Errorf("a missing region: %v", err)
	}
	// a marker in the middle of a line is not one
	if names, _ := Regions.Names("x = 1 -- @admin begin a\n"); names != nil {
		t.Errorf("a trailing comment read as a marker: %v", names)
	}
}
