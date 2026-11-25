package format

import (
	"slices"

	"alder/internal/diag"
)

// migration are the codes whose fixes Migrate applies.
var migration = map[string]bool{diag.ImportAlias: true, diag.FullPath: true, diag.HandlerRenamed: true}

// Migrate applies the migration fixes for file to src and reports whether anything changed: the
// resolver's import-qualifier fixes (E0204, E0205), the checker's rename of a write route's
// handler still named `update` (W0013, #308), and any fix marked Migration (a reference to a
// handler already renamed). bag is the diagnostics of the checked program, loaded from the same
// text as src. No other fix is applied: another diagnostic that shares an E0205's span (E0203 on
// `Table.Todo.Open`) carries its own fix, which formatting must not take. Identical fixes are
// applied once and a fix that overlaps one already taken is skipped, so the output is well formed
// whatever the bag holds; the next run picks up what was skipped (and a handler in a module that
// still has import errors, which the checker skips), and a migrated file has nothing left to fix.
func Migrate(bag *diag.Bag, file string, src []byte) ([]byte, bool, error) {
	var fixes []diag.Fix
	for _, d := range bag.Items {
		if d.Fix == nil || d.Span.File != file || !migration[d.Code] && !d.Fix.Migration {
			continue
		}
		fixes = append(fixes, *d.Fix)
	}
	slices.SortStableFunc(fixes, func(a, b diag.Fix) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Col != b.Col {
			return a.Col - b.Col
		}
		if a.EndLine != b.EndLine {
			return b.EndLine - a.EndLine // the wider fix first
		}
		return b.EndCol - a.EndCol
	})
	var kept []diag.Fix
	for _, f := range fixes {
		if n := len(kept); n > 0 {
			last := kept[n-1]
			if f == last || f.Line < last.EndLine || (f.Line == last.EndLine && f.Col < last.EndCol) {
				continue // a duplicate, or overlapping the fix before it
			}
		}
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		return src, false, nil
	}
	out, err := diag.Apply(string(src), kept)
	if err != nil {
		return nil, false, err
	}
	return []byte(out), out != string(src), nil
}
