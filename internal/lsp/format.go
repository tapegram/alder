package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/names"
	"alder/internal/scratch"
	"alder/internal/types"
)

// formattingReq answers textDocument/formatting: minimal edits that make the open buffer
// canonical. It first applies the file's import-qualifier fixes (E0204, E0205), as `alder fmt`
// does in a project, then formats the result. A buffer that does not parse, or whose formatting
// fails the safety check, yields null: the diagnostics already show the error.
func formattingReq(s *server, params json.RawMessage) (any, error) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	d := s.docs.get(rel)
	if d == nil || scratch.IsScratchFile(rel) {
		return nil, nil
	}
	src := s.migrated(rel, d)
	out, _, err := format.Source(rel, src)
	if errors.Is(err, format.ErrUnsafe) {
		fmt.Fprintf(s.o.Log, "alder lsp: formatting %s: %v\n", rel, err)
	}
	if err != nil || out == nil {
		return nil, nil
	}
	return lineEdits(d.Text, string(out)), nil
}

// migrated is d's text with its import-qualifier fixes applied; the text itself when migration is
// off: a project file does not parse (Program.Unparsed, the rule `alder fmt` uses, which covers an
// unparseable buffer), the project cannot be loaded, or there is nothing to fix. It reads the
// server's analysis when that was built from the open documents as they now are, and loads the
// program (names only, no checking) otherwise.
func (s *server) migrated(rel string, d *doc) []byte {
	text := []byte(d.Text)
	over, vers := s.docs.snapshot()
	var items []diag.Diagnostic
	if a := s.current(); a != nil && maps.Equal(a.versions, vers) {
		if a.prog.ParseFailed {
			return text
		}
		items = a.diags
	} else {
		std, err := types.LoadStd()
		if err != nil {
			return text
		}
		prog, bag, ioErrs := names.LoadOverlay(s.rootReal, std, over)
		if len(ioErrs) > 0 || prog.ParseFailed {
			return text
		}
		prog.Resolve(bag)
		items = bag.Items
	}
	out, _, err := format.Migrate(&diag.Bag{Items: items}, rel, text)
	if err != nil {
		fmt.Fprintf(s.o.Log, "alder lsp: migrating %s: %v\n", rel, err)
		return text
	}
	return out
}

// maxLCSCells bounds the line-level diff table; beyond it the differing middle is one edit.
const maxLCSCells = 4_000_000

// lineEdits are edits turning old into new, by a line-level diff: the common leading and trailing
// lines are left alone and an LCS over the lines between finds the rest. Positions are 0-based
// lines and UTF-8 byte offsets. It is never nil.
func lineEdits(old, new string) []TextEdit {
	edits := []TextEdit{}
	if old == new {
		return edits
	}
	a, b := splitLines(old), splitLines(new)
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	am, bm := a[pre:len(a)-suf], b[pre:len(b)-suf]

	// A lone \r is not a line break here either. pos is the position of the start of line i of a (or the end of the text for i == len(a)).
	pos := func(i int) Position {
		if i < len(a) || strings.HasSuffix(old, "\n") || len(a) == 0 {
			return Position{Line: i}
		}
		return Position{Line: len(a) - 1, Character: len(a[len(a)-1])}
	}
	emit := func(i, j int, text string) { // replace a[pre+i : pre+j]
		edits = append(edits, TextEdit{Range: Range{Start: pos(pre + i), End: pos(pre + j)}, NewText: text})
	}
	if len(am)*len(bm) > maxLCSCells {
		emit(0, len(am), strings.Join(bm, ""))
		return edits
	}

	// lcs[i][j] is the length of the LCS of am[i:] and bm[j:].
	lcs := make([][]int32, len(am)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(bm)+1)
	}
	for i := len(am) - 1; i >= 0; i-- {
		for j := len(bm) - 1; j >= 0; j-- {
			if am[i] == bm[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(am) || j < len(bm) {
		if i < len(am) && j < len(bm) && am[i] == bm[j] {
			i, j = i+1, j+1
			continue
		}
		si, sj := i, j // a differing hunk runs until the next common line
		for (i < len(am) || j < len(bm)) && !(i < len(am) && j < len(bm) && am[i] == bm[j]) {
			if j == len(bm) || (i < len(am) && lcs[i+1][j] >= lcs[i][j+1]) {
				i++
			} else {
				j++
			}
		}
		emit(si, i, strings.Join(bm[sj:j], ""))
	}
	return edits
}

// splitLines is s's lines, each keeping its newline; the last has none when s does not end in one.
// A lone \r is not a line break (as in the parser and LSP positions, only \n ends a line).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
