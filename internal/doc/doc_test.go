package doc

import (
	"slices"
	"strings"
	"testing"
)

const take = "The first n elements of a list.\n\nA list shorter than n comes back whole. An n of zero or less gives the empty list.\n\n    List.take 2 [1, 2, 3] == [1, 2]\n    List.take 0 [1, 2] == []\n\nBook: chapter 9, \"List\""

func TestParse(t *testing.T) {
	d := Parse(take)
	if d.Summary != "The first n elements of a list." || len(d.Details) != 1 || d.Book != `chapter 9, "List"` {
		t.Fatalf("%+v", d)
	}
	if len(d.Examples) != 2 || d.Examples[0] != (Example{"List.take 2 [1, 2, 3] == [1, 2]", 4}) || d.Examples[1].Line != 5 {
		t.Fatalf("%+v", d.Examples)
	}
}

func TestParse_Continuation(t *testing.T) {
	d := Parse("    List.map\n        (\\x -> x * 2)\n        [1, 2] == [2, 4]\n    List.length [] == 0")
	if len(d.Examples) != 2 || d.Examples[0].Source != "List.map\n    (\\x -> x * 2)\n    [1, 2] == [2, 4]" || d.Examples[1].Line != 3 || d.Summary != "" {
		t.Fatalf("%+v", d)
	}
}

func TestMarkdown(t *testing.T) {
	want := "The first n elements of a list.\n\nA list shorter than n comes back whole. An n of zero or less gives the empty list.\n\n```alder\nList.take 2 [1, 2, 3] == [1, 2]\nList.take 0 [1, 2] == []\n```\n\n*Book: chapter 9, \"List\"*"
	if got := Parse(take).Markdown(""); got != want {
		t.Fatalf("got %q", got)
	}
	if got := Parse("Sum.").Markdown(`chapter 9, "List"`); got != "Sum.\n\n*Book: chapter 9, \"List\"*" {
		t.Fatalf("fallback: %q", got)
	}
	if got := Parse("").Markdown(`chapter 9, "List"`); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

// Review Focus 1: an app comment written before the format keeps every line, in order.
func TestMarkdown_Lenient(t *testing.T) {
	got := Parse("lastPage bounds `page`\nto the last page\n\n    page 3\n\n- one\n- two").Markdown("")
	want := "lastPage bounds `page`\nto the last page\n\n```alder\npage 3\n```\n\n- one\n- two"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

// Review Focus 2: an example holding a fence gets a longer one.
func TestMarkdown_FenceInExample(t *testing.T) {
	got := Parse("Sum.\n\n    Text.length \"```\" == 3").Markdown("")
	if !strings.Contains(got, "````alder\nText.length \"```\" == 3\n````") {
		t.Fatalf("got %q", got)
	}
}

func TestProblems(t *testing.T) {
	const oneLine = "the summary is not one line ending in a full stop"
	cases := []struct {
		text string
		need Strict
		want []string
	}{
		{"", Strict{}, []string{"no doc comment"}},
		{"the first n code points", Strict{}, []string{oneLine}},
		{"Two lines\nof summary.", Strict{}, []string{oneLine}},
		{"Sum.", Strict{Example: true}, []string{"no example"}},
		{"Sum.", Strict{Book: true}, []string{"no Book line"}},
		{"Sum.\n\nBook: ch 9", Strict{}, []string{"the Book line is not `Book: chapter N, \"Heading\"` or `Book: appendix X, \"Heading\"`"}},
		{"Sum.\n\n    1 == 1\n\nMore prose.", Strict{}, []string{"prose follows the examples"}},
		{"Sum.\n\n    \t1 == 1", Strict{}, []string{"a tab in an example"}},
		{take, Strict{Example: true, Book: true}, nil},
	}
	for _, c := range cases {
		if got := Problems(c.text, c.need); !slices.Equal(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.text, got, c.want)
		}
	}
}

func TestParseBook(t *testing.T) {
	if k, n, h, ok := ParseBook(`appendix B, "Common mistakes"`); !ok || k != "appendix" || n != "B" || h != "Common mistakes" {
		t.Fatal(k, n, h, ok)
	}
	if _, _, _, ok := ParseBook("ch 9"); ok {
		t.Fatal("ch 9 parsed")
	}
}
