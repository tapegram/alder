package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"alder/internal/style"
)

// Every command of the tree is a row of the overall usage, on its own line, with its summary.
func TestHelp_ListsEveryCommand(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	usageTo(&out)
	text := out.String()
	if !strings.HasPrefix(text, "usage: alder <command> [arguments]\n\n") || strings.Contains(text, "|") {
		t.Errorf("usage:\n%s", text)
	}
	lines := strings.Split(text, "\n")
	for _, c := range commandTree {
		found := false
		for _, l := range lines {
			if strings.HasPrefix(l, "  "+c.Name+" ") && strings.HasSuffix(l, " "+c.Summary) {
				found = true
			}
		}
		if !found {
			t.Errorf("no row for %q with its summary %q:\n%s", c.Name, c.Summary, text)
		}
	}
	for _, l := range lines {
		if len(l) > 100 {
			t.Errorf("a %d-character line: %q", len(l), l)
		}
	}
	var help bytes.Buffer
	if !helpTo([]string{"help"}, &help) || help.String() != text {
		t.Errorf("`alder help` is not the overall usage:\n%s", help.String())
	}
}

func TestHelp_NamesBoldOnATerminal(t *testing.T) {
	t.Parallel()
	st := alwaysStyle()
	styled := usageText(st)
	if style.Strip(styled) != usageText(style.Plain) {
		t.Error("Strip(styled) != plain")
	}
	if !strings.Contains(styled, "  "+st.Bold("check")+"     type-check") {
		t.Errorf("names not bold, or the alignment moved:\n%s", styled)
	}
}

// The book's Commands block is `alder help`'s output.
func TestHelp_BookCommandsBlock(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../book/11-tooling.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "```\n"+usageText(style.Plain)+"```") {
		t.Errorf("book/11-tooling.md's Commands block is not `alder help`'s output:\n%s", usageText(style.Plain))
	}
}
