package generate

import (
	"fmt"
	"strings"
)

// UnifiedDiff is the unified diff (three lines of context) that turns a into b, the file at
// path (relative to the app) on both sides: `--- a/path`, `+++ b/path`, then the hunks. Equal
// texts give "".
func UnifiedDiff(path, a, b string) string {
	if a == b {
		return ""
	}
	al, bl := splitLines(a), splitLines(b)
	ops := diffLines(al, bl)
	const ctx = 3
	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", path, path)
	// group the ops into hunks: runs of changes with at most 2*ctx equal lines between them
	i := 0
	for i < len(ops) {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(0, i-ctx)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*ctx {
				end = min(len(ops), end+ctx)
				break
			}
			end = run
		}
		aStart, bStart, aN, bN := ops[start].ai, ops[start].bi, 0, 0
		var body strings.Builder
		for _, op := range ops[start:end] {
			switch op.kind {
			case ' ':
				aN++
				bN++
				body.WriteString(" " + al[op.ai] + "\n")
			case '-':
				aN++
				body.WriteString("-" + al[op.ai] + "\n")
			case '+':
				bN++
				body.WriteString("+" + bl[op.bi] + "\n")
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n%s", hunkRange(aStart, aN), hunkRange(bStart, bN), body.String())
		i = end
	}
	return out.String()
}

// LineEdit replaces lines [Start, End) of a text (0-based) with Text: whole lines, each ending
// in a newline ("" removes them; Start == End inserts before line Start).
type LineEdit struct {
	Start, End int
	Text       string
}

// LineEdits are the edits that turn before into after, one per run of changed lines of the
// diff (no context), in order: an editor applies them as text edits, so a filled hole and the
// import it added are two edits, and the lines between them are left as they are. Both texts
// are taken to end in a newline (as every formatted file does).
func LineEdits(before, after string) []LineEdit {
	if before == after {
		return nil
	}
	al, bl := splitLines(before), splitLines(after)
	ops := diffLines(al, bl)
	var out []LineEdit
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		e := LineEdit{Start: ops[i].ai, End: ops[i].ai}
		var text strings.Builder
		for ; i < len(ops) && ops[i].kind != ' '; i++ {
			if ops[i].kind == '-' {
				e.End = ops[i].ai + 1
			} else {
				text.WriteString(bl[ops[i].bi] + "\n")
			}
		}
		e.Text = text.String()
		out = append(out, e)
	}
	return out
}

// hunkRange is a hunk header's range: the first line (1-based) and the count, "n" alone when
// the count is 1, and the line before when the count is 0.
func hunkRange(start, n int) string {
	switch n {
	case 0:
		return fmt.Sprintf("%d,0", start)
	case 1:
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

// splitLines splits s into lines, without a last empty one for a final newline.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffOp is one line of a diff: kind ' ', '-' or '+', with the line's index in a (ai) and in b
// (bi); for '-' bi is where b stands, for '+' ai is where a stands.
type diffOp struct {
	kind   byte
	ai, bi int
}

// diffLines is a shortest edit script from a to b, by the longest common subsequence.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			ops = append(ops, diffOp{' ', i, j})
			i++
			j++
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, diffOp{'-', i, j})
			i++
		default:
			ops = append(ops, diffOp{'+', i, j})
			j++
		}
	}
	return ops
}
