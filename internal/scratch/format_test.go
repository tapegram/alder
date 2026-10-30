package scratch

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"watch spacing", "> 1   +1\n", "> 1 + 1\n"},
		{"adjacent watches stay adjacent", "> [1,2]\n> 3\n", "> [1, 2]\n> 3\n"},
		{"separated watches stay separated", "> 1\n\n\n> 2\n", "> 1\n\n> 2\n"},
		{"definition and watch",
			"double : Int -> Int\ndouble n = n*2\n> double 21\n",
			"double : Int -> Int\ndouble n = n * 2\n\n> double 21\n"},
		{"in header", "in Todo\n\n> 1+1\n", "in Todo\n\n> 1 + 1\n"},
		{"comment before the in header", "-- notes\nin Todo\n> 1+1\n", "-- notes\nin Todo\n\n> 1 + 1\n"},
		{"leading comment without header", "-- A scratch file.\n\n> 1+1\n", "-- A scratch file.\n\n> 1 + 1\n"},
		{"module header kept", "module Probe\n\n> 1+1\n", "module Probe\n\n> 1 + 1\n"},
		{"watch comment", "> 1 + 1  -- two\n", "> 1 + 1 -- two\n"},
		{"multi-line watch",
			"> List.map (\\aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa -> aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa + 1111111111111111111111111) [1,2,3,4,5,6,7,8,9,10]\n",
			"> List.map (\\aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ->\n  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa + 1111111111111111111111111\n) [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]\n"},
		{"empty file", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, ds, err := Format("scratch.alder", []byte(tc.in))
			if err != nil || len(ds) > 0 {
				t.Fatalf("err %v diags %+v", err, ds)
			}
			if string(out) != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", out, tc.want)
			}
			again, _, err := Format("scratch.alder", out)
			if err != nil || string(again) != string(out) {
				t.Fatalf("not idempotent: %q (%v)", again, err)
			}
		})
	}
}

func TestFormatLongWatchBreaksAfterTheName(t *testing.T) {
	// a watch whose expression the printer breaks right after `itK =` still starts on its `>` line
	in := "> \"" + strings.Repeat("a", 120) + "\"\n"
	out, ds, err := Format("scratch.alder", []byte(in))
	if err != nil || len(ds) > 0 {
		t.Fatalf("err %v diags %+v", err, ds)
	}
	if !strings.HasPrefix(string(out), "> \"aaa") {
		t.Fatalf("got %q", out)
	}
	f, pds := ParseFile("scratch.alder", string(out))
	if len(pds) > 0 || len(f.Watches) != 1 {
		t.Fatalf("output %q: watches %+v diags %+v", out, f.Watches, pds)
	}
}

func TestFormatRefusesBrokenFiles(t *testing.T) {
	for _, in := range []string{
		"> 1 +\n",                  // does not parse
		">!Db.insert x\n",          // ParseFile's own error
		"it1 = 2\n> 1\n",           // a reserved name
		"in Todo\nin Todo\n> 1\n",  // a header out of place
	} {
		out, ds, err := Format("scratch.alder", []byte(in))
		if err != nil || out != nil || len(ds) == 0 {
			t.Errorf("%q: out %q diags %+v err %v", in, out, ds, err)
		}
		for _, d := range ds {
			if d.Span.File != "scratch.alder" || d.Span.Start.Line < 1 {
				t.Errorf("%q: diagnostic not in the file: %+v", in, d)
			}
		}
	}
}
