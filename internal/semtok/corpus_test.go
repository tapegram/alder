package semtok

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/syntax"
)

// TestInvariants_Corpus: every .alder in the repo, broken ones included, gives sorted,
// non-overlapping tokens inside their lines and the legend (Review Focus 2). Scratch files go
// through Scratch, by the server's rule (scratch.IsScratchFile: a scratch name not under app/).
func TestInvariants_Corpus(t *testing.T) {
	n, scratches := 0, 0
	roots := []string{"../../examples", "../../testdata", "../../stdlib", "../../internal/scaffold/files", "../../tree-sitter-alder/test"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".alder") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			n++
			rel := filepath.ToSlash(path)
			if syntax.IsScratchName(rel) && !strings.Contains(rel, "/app/") {
				scratches++
				checkInvariants(t, path, string(b), Scratch(path, string(b)))
				return nil
			}
			checkInvariants(t, path, string(b), Module(path, b))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if n < 500 {
		t.Fatalf("only %d sources", n)
	}
	t.Logf("%d sources, %d scratch", n, scratches)
	if scratches < 2 {
		t.Fatalf("only %d scratch files", scratches)
	}
}
