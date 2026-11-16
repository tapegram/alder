package stdref

import (
	"strings"
	"testing"
)

func TestMarkdown(t *testing.T) {
	md, err := Markdown()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"## List\n", "### `List.take`\n", "```alder fragment\nList.take 2 [1, 2, 3] == [1, 2]"} {
		if !strings.Contains(md, w) {
			t.Errorf("missing %q", w)
		}
	}
	if strings.Index(md, "## Dict\n") > strings.Index(md, "## List\n") || !strings.Contains(md, "## Dict\n") {
		t.Error("## Dict must come before ## List")
	}
	if strings.Contains(md, "*Book:") {
		t.Error("book lines must be left out")
	}
}
