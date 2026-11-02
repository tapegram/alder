package testrun

import (
	"reflect"
	"testing"
)

// stream is three lines of `alder test --json`: a passing test, a failing one with left and
// right, and a property test with its runs, seed and case.
var stream = []string{
	`{"module":"Books","name":"lend a book","file":"app/Books.al","line":10,"col":1,"ok":true,"trace":[],"ms":1.2}`,
	`{"module":"Books","name":"lend (twice)","file":"app/Books.al","line":20,"col":1,"ok":false,"left":"Lent 2","right":"Lent 1","note":"a book lends once","trace":["Books.lend app/Books.al:14"],"ms":3.4}`,
	`{"module":"Search","name":"finds by title","file":"app/Search.al","line":5,"col":1,"ok":true,"trace":[],"ms":40.5,"property":{"runs":100,"seed":4242,"run":0,"shrinks":0,"args":[{"name":"title","type":"String","value":"\"a\""}],"case":"finds by title \"a\""}}`,
}

func TestParse_Stream(t *testing.T) {
	var r Run
	for _, l := range stream {
		Parse(l, &r)
	}
	Parse("", &r)
	if len(r.Results) != 3 {
		t.Fatalf("%d results, want 3: %+v", len(r.Results), r.Results)
	}
	failed := 0
	for _, res := range r.Results {
		if !res.OK {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("%d failures, want 1", failed)
	}
	f := r.Results[1]
	want := Result{Module: "Books", Name: "lend (twice)", File: "app/Books.al", Line: 20, Col: 1,
		Left: "Lent 2", Right: "Lent 1", Note: "a book lends once", Trace: []string{"Books.lend app/Books.al:14"}, Ms: 3.4}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("failure = %+v, want %+v", f, want)
	}
	p := r.Results[2].Property
	if p == nil {
		t.Fatal("the property test has no property")
	}
	wantP := Property{Runs: 100, Seed: 4242, Args: []Arg{{Name: "title", Type: "String", Value: `"a"`}}, Case: `finds by title "a"`}
	if !reflect.DeepEqual(*p, wantP) {
		t.Errorf("property = %+v, want %+v", *p, wantP)
	}
	if r.Error != "" || r.Diagnostics != nil || r.Score != nil {
		t.Errorf("a stream of tests set more than results: %+v", r)
	}
}

func TestParse_Diagnostics(t *testing.T) {
	var r Run
	Parse(`{"diagnostics":[{"code":"E0002","severity":"error","file":"app/Books.al","line":14,"col":12,"endLine":14,"endCol":20,"message":"unknown name `+"`findBook`"+`"}]}`, &r)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Message != "unknown name `findBook`" || r.Diagnostics[0].Line != 14 {
		t.Fatalf("diagnostics = %+v", r.Diagnostics)
	}
	if len(r.Results) != 0 {
		t.Errorf("a diagnostics object made results: %+v", r.Results)
	}
	var e Run
	Parse(`{"error":"bad -run: missing closing )"}`, &e)
	if e.Error != "bad -run: missing closing )" || len(e.Results) != 0 {
		t.Errorf("error object gives %+v", e)
	}
}

func TestParse_Mutate(t *testing.T) {
	var r Run
	Parse(`{"mutants":[`+
		`{"id":1,"module":"Books","file":"app/Books.al","decl":"lend","line":14,"col":9,"kind":"arith","before":"n + 1","after":"n - 1","killed":true,"by":"Books \"lend a book\"","note":""},`+
		`{"id":2,"module":"Books","file":"app/Books.al","decl":"lend","line":16,"col":5,"kind":"cond","before":"n > 0","after":"n >= 0","killed":false,"by":"","note":""}]}`, &r)
	Parse(`{"score":0.5,"killed":1,"survived":1,"suppressed":2,"skipped":3}`, &r)
	if r.Score == nil || *r.Score != (Score{Score: 0.5, Killed: 1, Survived: 1, Suppressed: 2, Skipped: 3}) {
		t.Fatalf("score = %+v", r.Score)
	}
	var survived []Mutant
	for _, m := range r.Mutants {
		if !m.Killed {
			survived = append(survived, m)
		}
	}
	want := Mutant{ID: 2, Module: "Books", File: "app/Books.al", Decl: "lend", Line: 16, Col: 5, Kind: "cond", Before: "n > 0", After: "n >= 0"}
	if len(r.Mutants) != 2 || len(survived) != 1 || survived[0] != want {
		t.Errorf("mutants = %+v", r.Mutants)
	}
}

func TestArgs_One(t *testing.T) {
	one := &Result{Module: "Books", Name: "lend (twice)"}
	if got, want := Args(one, false), []string{"test", "--json", "-run", `^Books "lend \(twice\)"$`}; !reflect.DeepEqual(got, want) {
		t.Errorf("Args(one) = %q, want %q", got, want)
	}
	if got, want := Args(nil, false), []string{"test", "--json"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Args(nil) = %q, want %q", got, want)
	}
	if got, want := Args(nil, true), []string{"test", "--json", "--mutate"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Args(nil, mutate) = %q, want %q", got, want)
	}
}

func TestParse_BadDiagnostic(t *testing.T) {
	for _, line := range []string{
		`{"diagnostics":5}`,
		`{"diagnostics":[{"line":"ten"}]}`,
		`{"mutants":{"id":1}}`,
	} {
		var r Run
		Parse(line, &r)
		if r.Error == "" {
			t.Errorf("Parse(%s) left no error", line)
		}
		if r.Diagnostics != nil || r.Mutants != nil {
			t.Errorf("Parse(%s) kept a half-read value: %+v %+v", line, r.Diagnostics, r.Mutants)
		}
	}
}
