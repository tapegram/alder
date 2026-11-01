package look

import (
	"reflect"
	"strconv"
	"testing"
)

func TestMark(t *testing.T) {
	cases := map[string]string{
		"ok":      "\x1b[32mok\x1b[0m",
		"FAIL":    "\x1b[1;31mFAIL\x1b[0m",
		"error":   "\x1b[1;31merror\x1b[0m",
		"warning": "\x1b[33mwarning\x1b[0m",
		"SKIPPED": "\x1b[33mSKIPPED\x1b[0m",
		"other":   "other",
	}
	for in, want := range cases {
		if got := Mark(in); got != want {
			t.Errorf("Mark(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		200: "200",
		302: "\x1b[2m302\x1b[0m",
		404: "\x1b[33m404\x1b[0m",
		500: "\x1b[1;31m500\x1b[0m",
	}
	for code, want := range cases {
		if got := StatusClass(code)(strconv.Itoa(code)); got != want {
			t.Errorf("StatusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestColumns(t *testing.T) {
	rows := [][]string{{"GET", "/", "200"}, {"POST", "/login", "302"}}
	want := []string{"GET   /       200", "POST  /login  302"}
	if got := Columns(rows, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := Columns([][]string{{"a", ""}, {"bb", "c"}}, 1); !reflect.DeepEqual(got, []string{"a", "bb c"}) {
		t.Errorf("trailing spaces: %q", got)
	}
}
