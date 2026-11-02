package look

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
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

// A styled cell, a wide rune and a plain cell start the next column at the same cell.
func TestColumns_Styled(t *testing.T) {
	rows := [][]string{{"ok", "a"}, {Palette.Green("ok"), "b"}, {"界界", "c"}, {Palette.BoldRed("FAIL"), "d"}}
	for i, l := range Columns(rows, 2) {
		if got := ansi.StringWidth(l[:strings.LastIndex(l, rows[i][1])]); got != 4+2 {
			t.Errorf("row %d: the second column starts at cell %d, want 6: %q", i, got, l)
		}
	}
}
