package search

import (
	"strings"
	"testing"
)

func hitNames(r Response) []string {
	out := []string{}
	for _, h := range r.Results {
		out = append(out, h.Entry.Module+"."+h.Entry.Name)
	}
	return out
}

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

func TestName_Exact(t *testing.T) {
	r := std(t).Search("split", Options{})
	if r.Mode != "name" {
		t.Fatalf("mode %q", r.Mode)
	}
	if got := hitNames(r); len(got) == 0 || got[0] != "Text.split" {
		t.Fatalf("got %v", got)
	}
}

func TestName_Module(t *testing.T) {
	ix := std(t)
	r := ix.Search("Text.", Options{Limit: -1})
	var want []string
	for _, e := range ix.Entries {
		if e.Module == "Text" {
			want = append(want, "Text."+e.Name)
		}
	}
	if got := hitNames(r); strings.Join(got, ",") != strings.Join(want, ",") || len(want) == 0 {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, q := range []string{"Text.sp", "text.SP", "Tex.sp"} {
		if got := hitNames(ix.Search(q, Options{})); len(got) == 0 || got[0] != "Text.split" {
			t.Errorf("%q: got %v", q, got)
		}
	}
}

func TestName_Tiers(t *testing.T) {
	ix := std(t)
	got := hitNames(ix.Search("map", Options{Limit: -1}))
	lm, mm, fm := indexOf(got, "List.map"), indexOf(got, "Maybe.map"), indexOf(got, "List.filterMap")
	if lm < 0 || mm < 0 || fm < 0 || lm > fm || mm > fm {
		t.Fatalf("order wrong: %v", got)
	}
	for _, n := range got[:fm] {
		if !strings.Contains(strings.ToLower(n[strings.Index(n, ".")+1:]), "map") {
			t.Errorf("%s before filterMap does not contain map", n)
		}
	}
	if got := hitNames(ix.Search("fMap", Options{})); indexOf(got, "List.filterMap") < 0 {
		t.Errorf("fMap: %v", got)
	}
}

func TestName_ModuleOnly(t *testing.T) {
	got := hitNames(std(t).Search("Maybe", Options{Limit: -1}))
	if len(got) == 0 || got[0] != "Prelude.Maybe" {
		t.Fatalf("first %v", got)
	}
	if indexOf(got, "Maybe.map") < 0 {
		t.Errorf("Maybe.map missing")
	}
}

func TestSearch_Limit(t *testing.T) {
	ix := std(t)
	if n := len(ix.Search("Text.", Options{Limit: 3}).Results); n != 3 {
		t.Errorf("limit 3: %d", n)
	}
	all := len(ix.Search("Text.", Options{Limit: -1}).Results)
	if all <= 3 {
		t.Errorf("all: %d", all)
	}
	if n := len(ix.Search("a", Options{}).Results); n != 50 {
		t.Errorf("default cap: %d", n)
	}
}

func TestSearch_Source(t *testing.T) {
	if r := std(t).Search("map", Options{Source: "app"}); len(r.Results) != 0 {
		t.Errorf("got %v", hitNames(r))
	}
}

func TestSearch_Empty(t *testing.T) {
	for _, q := range []string{"", "   "} {
		r := std(t).Search(q, Options{})
		if r.Mode != "name" || r.Results == nil || len(r.Results) != 0 {
			t.Errorf("%q: %+v", q, r)
		}
	}
}
