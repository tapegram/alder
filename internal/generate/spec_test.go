package generate

import (
	"errors"
	"reflect"
	"testing"
)

var testPage = &Spec{
	ID:   "new page",
	Args: []Field{{Name: "module", Kind: ArgModule, Required: true}, {Name: "path", Kind: ArgRoute, Required: true}},
	Flags: []Field{
		{Name: "show", Kind: ArgTable}, {Name: "from", Kind: ArgTable, Greedy: true},
		{Name: "param", Kind: ArgText, Variadic: true}, {Name: "blank", Kind: ArgBool},
	},
}

var testTable = &Spec{
	ID:    "new table",
	Args:  []Field{{Name: "name", Kind: ArgModule, Required: true}, {Name: "fields", Kind: ArgFieldList, Variadic: true, Table: true}},
	Flags: []Field{{Name: "silo", Kind: ArgBool}},
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		spec *Spec
		argv []string
		want Sets
		cli  CLIFlags
		err  string
	}{
		{"interleaved", testPage, []string{"Dash", "--from", "Entry", "/dash"}, Sets{"module": {"Dash"}, "path": {"/dash"}, "from": {"Entry"}}, CLIFlags{Dir: "."}, ""},
		{"greedy stops at flag", testPage, []string{"--from", "A", "B", "--blank", "X", "/x"}, Sets{"module": {"X"}, "path": {"/x"}, "from": {"A", "B"}, "blank": {""}}, CLIFlags{Dir: "."}, ""},
		{"equals and repeat", testPage, []string{"X", "/x/{id}", "--param=id:Int", "-param", "n:Text", "--show", "Post"}, Sets{"module": {"X"}, "path": {"/x/{id}"}, "param": {"id:Int", "n:Text"}, "show": {"Post"}}, CLIFlags{Dir: "."}, ""},
		{"cli flags", testPage, []string{"X", "/x", "--json", "--dry-run", "--dir", "d"}, Sets{"module": {"X"}, "path": {"/x"}}, CLIFlags{Dir: "d", JSON: true, DryRun: true}, ""},
		{"variadic", testTable, []string{"Tag", "name:Text", "n:Int=0", "--silo"}, Sets{"name": {"Tag"}, "fields": {"name:Text", "n:Int=0"}, "silo": {""}}, CLIFlags{Dir: "."}, ""},
		{"variadic empty", testTable, []string{"Tag"}, Sets{"name": {"Tag"}}, CLIFlags{Dir: "."}, ""},
		{"unknown flag", testPage, []string{"X", "/x", "--nope"}, nil, CLIFlags{}, "unknown flag --nope"},
		{"missing value", testPage, []string{"X", "/x", "--show"}, nil, CLIFlags{}, "--show needs a value"},
		{"bool with value", testPage, []string{"X", "/x", "--blank=1"}, nil, CLIFlags{}, "unknown flag --blank=1"},
		{"too few", testPage, []string{"X"}, nil, CLIFlags{}, ErrArity.Error()},
		{"too many", testPage, []string{"X", "/x", "Y"}, nil, CLIFlags{}, ErrArity.Error()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sets, cli, err := ParseArgs(c.spec, c.argv)
			if c.err != "" {
				if err == nil || err.Error() != c.err {
					t.Fatalf("err = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sets, c.want) || cli != c.cli {
				t.Fatalf("got %v %+v, want %v %+v", sets, cli, c.want, c.cli)
			}
		})
	}
	if _, _, err := ParseArgs(testPage, []string{"X"}); !errors.Is(err, ErrArity) {
		t.Fatalf("arity error is not ErrArity: %v", err)
	}
}

func TestSelect(t *testing.T) {
	a := &Spec{ID: "edit table"}
	b := &Spec{ID: "edit auth"}
	specs := []*Spec{a, b}
	if s, rest := Select("edit", []string{"--to", "Home", "auth"}, specs); s != b || !reflect.DeepEqual(rest, []string{"--to", "Home"}) {
		t.Fatalf("got %v %v", s, rest)
	}
	if s, rest := Select("edit", []string{"table", "Post", "drop-field", "x"}, specs); s != a || !reflect.DeepEqual(rest, []string{"Post", "drop-field", "x"}) {
		t.Fatalf("got %v %v", s, rest)
	}
	if s, _ := Select("edit", []string{"nope"}, specs); s != nil {
		t.Fatal("selected an unknown subcommand")
	}
}
