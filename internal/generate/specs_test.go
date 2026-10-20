package generate

import (
	"strings"
	"testing"
)

func TestSpecs_IDsAndShape(t *testing.T) {
	want := []string{"new page", "new table", "new form", "new msg", "new capability", "new admin", "new auth", "new type", "new decl", "new test", "new silo"}
	var got []string
	for _, s := range Specs() {
		if strings.HasPrefix(s.ID, "new ") {
			got = append(got, s.ID)
		}
		if s.Summary == "" || s.Run == nil {
			t.Errorf("%s: missing Summary or Run", s.ID)
		}
		for i, a := range s.Args {
			if a.Variadic && i != len(s.Args)-1 {
				t.Errorf("%s: variadic arg %s is not last", s.ID, a.Name)
			}
		}
		for _, f := range s.Flags {
			if f.Summary == "" {
				t.Errorf("%s: flag --%s has no summary", s.ID, f.Name)
			}
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("new specs = %v", got)
	}
}

func TestSpecs_NewDeclDryRun(t *testing.T) {
	dir := scaffoldApp(t, nil)
	s := SpecByID("new decl")
	res, err := s.Run(Sets{"module": {"Home"}, "name": {"answer"}, "type": {"Int"}}, App{Dir: dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Written || len(res.Files) != 1 || res.Files[0].Path != "app/Home.alder" || !strings.Contains(*res.Files[0].After, "answer : Int") {
		t.Fatalf("dry run: %+v", res.Files)
	}
}

func TestSpecs_FieldParseErrorIsRefusal(t *testing.T) {
	dir := scaffoldApp(t, nil)
	_, err := SpecByID("new table").Run(Sets{"name": {"Tag"}, "fields": {"bad"}}, App{Dir: dir}, true)
	if _, ok := err.(*Refusal); !ok {
		t.Fatalf("want a *Refusal, got %T %v", err, err)
	}
}

func TestSpecs_AllIDs(t *testing.T) {
	want := "new page,new table,new form,new msg,new capability,new admin,new auth,new type,new decl,new test,new silo," +
		"edit table,edit page,edit decl,edit auth,edit session,remove page,remove table,regen admin"
	var ids []string
	for _, s := range Specs() {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != want {
		t.Fatalf("ids = %v", ids)
	}
}

func TestSpecs_EditTableSlug(t *testing.T) {
	cases := map[string]Sets{
		"add_title_text": {"table": {"Post"}, "op": {"add-field"}, "args": {"titleText:Text"}},
		"rename_title":   {"table": {"Post"}, "op": {"rename-field"}, "args": {"title", "heading"}},
		"drop_title":     {"table": {"Post"}, "op": {"drop-field"}, "args": {"title"}},
	}
	for want, sets := range cases {
		if got := editTableSlug(sets); got != want {
			t.Errorf("%v: slug %q, want %q", sets, got, want)
		}
	}
}

func TestSpecs_NewPageFileTypeIsQuoted(t *testing.T) {
	dir := scaffoldApp(t, nil)
	res, err := SpecByID("new page").Run(Sets{"module": {"Reports.Csv"}, "path": {"/reports.csv"}, "file": {"true"}, "type": {`text/csv"`}}, App{Dir: dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if f.After != nil && strings.Contains(*f.After, "File.text") {
			if !strings.Contains(*f.After, `contentType: "text/csv\""`) {
				t.Fatalf("--type is escaped in the string literal:\n%s", *f.After)
			}
			return
		}
	}
	t.Fatalf("no file page in %+v", res.Files)
}
