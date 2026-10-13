package book

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"alder/internal/elements"
	"alder/rt"
)

// Appendix C documents the `alder-ui` elements the compiler embeds. Its tables are written by
// hand, so this test holds them to the manifest (rt/al/custom-elements.json, vendored by
// scripts/vendor-al.sh): every element has a section headed "### `al-x`", in the manifest's order; its
// attribute table lists exactly the manifest's attributes, in order, with the same type and
// default (or the line "No attributes."); its "Slots:" line lists the slots in order ("default"
// for the default slot, "none" for no slot); and the chapter names the vendored VERSION.
func TestAppendixC_Manifest(t *testing.T) {
	md, err := os.ReadFile("C-design-system.md")
	if err != nil {
		t.Fatal(err)
	}
	m, version := builtinManifest(t)
	for _, msg := range manifestDrift(string(md), m, version) {
		t.Error("book/C-design-system.md: " + msg)
	}
}

// The drift check fails on each kind of difference.
func TestAppendixC_ManifestDrift(t *testing.T) {
	b, err := os.ReadFile("C-design-system.md")
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	m, version := builtinManifest(t)
	for _, c := range []struct{ name, old, new, want string }{
		{"a type", "| `size` | `\"sm\" \\| \"md\" \\| \"lg\"` | `\"md\"` |", "| `size` | `\"sm\" \\| \"lg\"` | `\"md\"` |",
			"`al-button` attribute `size`: the chapter's type is `\"sm\" | \"lg\"`, the manifest's `\"sm\" | \"md\" | \"lg\"`"},
		{"a default", "| `gap` | `\"sm\" \\| \"md\" \\| \"lg\"` | `\"md\"` |", "| `gap` | `\"sm\" \\| \"md\" \\| \"lg\"` | `\"lg\"` |",
			"`al-stack` attribute `gap`: the chapter's default is `\"lg\"`, the manifest's `\"md\"`"},
		{"a name", "| `heading` | `string` |", "| `title` | `string` |",
			"`al-alert` attributes: the chapter has `variant`, `title`, `dismissible`, the manifest `variant`, `heading`, `dismissible`"},
		{"a slot", "Slots: `prev`, default, `next`.", "Slots: `prev`, `next`.",
			"`al-pagination` slots: the chapter has `prev`, `next`, the manifest `prev`, default, `next`"},
		{"an element", "### `al-stack`", "### `al-row`",
			"the chapter documents `al-row`, which the manifest does not declare"},
		{"the version", "**" + version + "**", "**v0.0.9**", "the chapter does not name the vendored version " + version},
	} {
		if !strings.Contains(md, c.old) {
			t.Fatalf("%s: the chapter lacks %q", c.name, c.old)
		}
		msgs := manifestDrift(strings.Replace(md, c.old, c.new, 1), m, version)
		if !slices.ContainsFunc(msgs, func(s string) bool { return strings.Contains(s, c.want) }) {
			t.Errorf("%s: want %q among %q", c.name, c.want, msgs)
		}
	}
}

// builtinManifest is the embedded manifest and the vendored VERSION.
func builtinManifest(t *testing.T) (*elements.Manifest, string) {
	t.Helper()
	data, err := rt.Files.ReadFile("al/custom-elements.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := elements.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	v, err := rt.Files.ReadFile("al/VERSION")
	if err != nil {
		t.Fatal(err)
	}
	return m, strings.TrimSpace(string(v))
}

var (
	elementHeadingRE = regexp.MustCompile("^### `(al-[a-z-]+)`$")
	manifestTagOrder = regexp.MustCompile(`"tagName":\s*"([^"]+)"`)
)

// manifestDrift compares the element sections of the appendix md with manifest m.
func manifestDrift(md string, m *elements.Manifest, version string) []string {
	var msgs []string
	if !strings.Contains(md, "**"+version+"**") {
		msgs = append(msgs, "the chapter does not name the vendored version "+version+" (in bold)")
	}
	type section struct {
		tag   string
		lines []string
	}
	var sections []*section
	var cur *section
	for _, line := range strings.Split(md, "\n") {
		if g := elementHeadingRE.FindStringSubmatch(line); g != nil {
			cur = &section{tag: g[1]}
			sections = append(sections, cur)
			continue
		}
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			cur = nil
		}
		if cur != nil {
			cur.lines = append(cur.lines, line)
		}
	}
	var tags []string
	for _, s := range sections {
		tags = append(tags, s.tag)
	}
	var want []string
	data, _ := rt.Files.ReadFile("al/custom-elements.json")
	for _, g := range manifestTagOrder.FindAllStringSubmatch(string(data), -1) {
		if m.Lookup(g[1]) != nil {
			want = append(want, g[1])
		}
	}
	for _, tag := range tags {
		if m.Lookup(tag) == nil {
			msgs = append(msgs, fmt.Sprintf("the chapter documents `%s`, which the manifest does not declare", tag))
		}
	}
	for _, tag := range want {
		if !slices.Contains(tags, tag) {
			msgs = append(msgs, fmt.Sprintf("the manifest declares `%s`, which the chapter has no section for", tag))
		}
	}
	if len(msgs) == 0 && !slices.Equal(tags, want) {
		msgs = append(msgs, fmt.Sprintf("the sections are in the order %s, the manifest's is %s", strings.Join(tags, ", "), strings.Join(want, ", ")))
	}
	for _, s := range sections {
		el := m.Lookup(s.tag)
		if el == nil {
			continue
		}
		msgs = append(msgs, sectionDrift(s.tag, s.lines, el)...)
	}
	return msgs
}

// sectionDrift compares one element's section with its manifest entry.
func sectionDrift(tag string, lines []string, el *elements.Element) []string {
	var msgs []string
	type row struct{ name, typ, def string }
	var rows []row
	inTable, noAttrs := false, false
	var slotsLine string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "| Attribute | Type | Default |"):
			inTable = true
		case inTable && strings.HasPrefix(l, "|---"):
		case inTable && strings.HasPrefix(l, "| "):
			cells := splitRow(l)
			if len(cells) < 3 {
				msgs = append(msgs, fmt.Sprintf("`%s`: a table row with fewer than three cells: %s", tag, l))
				continue
			}
			def := unquote(cells[2])
			if def == "—" {
				def = ""
			}
			rows = append(rows, row{unquote(cells[0]), unquote(cells[1]), def})
		case inTable:
			inTable = false
		case l == "No attributes.":
			noAttrs = true
		case strings.HasPrefix(l, "Slots: "):
			slotsLine = l
		}
	}
	var gotNames, wantNames []string
	for _, r := range rows {
		gotNames = append(gotNames, "`"+r.name+"`")
	}
	for _, a := range el.Attrs {
		wantNames = append(wantNames, "`"+a.Name+"`")
	}
	switch {
	case len(rows) == 0 && !noAttrs:
		msgs = append(msgs, fmt.Sprintf("`%s` has no attribute table and no line \"No attributes.\"", tag))
	case noAttrs && len(el.Attrs) > 0:
		msgs = append(msgs, fmt.Sprintf("`%s` says \"No attributes.\", the manifest has %s", tag, strings.Join(wantNames, ", ")))
	case !slices.Equal(gotNames, wantNames):
		msgs = append(msgs, fmt.Sprintf("`%s` attributes: the chapter has %s, the manifest %s", tag, strings.Join(gotNames, ", "), strings.Join(wantNames, ", ")))
	default:
		for i, r := range rows {
			a := el.Attrs[i]
			if r.typ != strings.TrimSpace(a.Type) {
				msgs = append(msgs, fmt.Sprintf("`%s` attribute `%s`: the chapter's type is `%s`, the manifest's `%s`", tag, a.Name, r.typ, strings.TrimSpace(a.Type)))
			}
			if r.def != a.Default {
				msgs = append(msgs, fmt.Sprintf("`%s` attribute `%s`: the chapter's default is `%s`, the manifest's `%s`", tag, a.Name, r.def, a.Default))
			}
		}
	}
	var wantSlots []string
	for _, s := range el.Slots {
		if s == "" {
			wantSlots = append(wantSlots, "default")
		} else {
			wantSlots = append(wantSlots, "`"+s+"`")
		}
	}
	if len(wantSlots) == 0 {
		wantSlots = []string{"none"}
	}
	var gotSlots []string
	if slotsLine == "" {
		msgs = append(msgs, fmt.Sprintf("`%s` has no \"Slots:\" line", tag))
	} else {
		list := strings.TrimSuffix(strings.TrimPrefix(slotsLine, "Slots: "), ".")
		for _, s := range strings.Split(list, ",") {
			s = strings.TrimSpace(s)
			if i := strings.Index(s, " ("); i >= 0 { // "default (the `<table>`)"
				s = s[:i]
			}
			gotSlots = append(gotSlots, s)
		}
		if !slices.Equal(gotSlots, wantSlots) {
			msgs = append(msgs, fmt.Sprintf("`%s` slots: the chapter has %s, the manifest %s", tag, strings.Join(gotSlots, ", "), strings.Join(wantSlots, ", ")))
		}
	}
	return msgs
}

// splitRow is the cells of a Markdown table row, `\|` read as a `|` inside a cell.
func splitRow(l string) []string {
	l = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(l), "|"), "|")
	var cells []string
	var b strings.Builder
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\' && i+1 < len(l) && l[i+1] == '|':
			b.WriteByte('|')
			i++
		case l[i] == '|':
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(l[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

// unquote is a cell's text without its surrounding backticks.
func unquote(cell string) string {
	if len(cell) >= 2 && strings.HasPrefix(cell, "`") && strings.HasSuffix(cell, "`") {
		return cell[1 : len(cell)-1]
	}
	return cell
}
