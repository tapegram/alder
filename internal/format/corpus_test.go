package format

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/book"
	"alder/internal/syntax"
)

// skipTiming skips a wall-clock test where timing is meaningless or noisy: -short and -race.
func skipTiming(t *testing.T) {
	t.Helper()
	if testing.Short() || raceEnabled {
		t.Skip("timing test: skipped under -short and -race")
	}
}

// corpusFiles lists every .alder file under root, skipping build/, experiments/ and dot-dirs.
func corpusFiles(t testing.TB, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			n := d.Name()
			if p != root && (n == "build" || n == "experiments" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".alder") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

type corpusInput struct {
	name string
	src  []byte
}

// corpusInputs is the files of examples/ and testdata/corpus/ plus the book's whole-module blocks.
// stdlib/ is excluded: its files parse only in signature mode, which Source does not handle.
func corpusInputs(t testing.TB) []corpusInput {
	t.Helper()
	var in []corpusInput
	for _, root := range []string{"../../examples", "../../testdata/corpus"} {
		for _, f := range corpusFiles(t, root) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			in = append(in, corpusInput{f, src})
		}
	}
	chapters, err := filepath.Glob("../../book/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range chapters {
		snips, err := book.Extract(ch)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range snips {
			if book.Classify(s) == book.Module {
				in = append(in, corpusInput{fmt.Sprintf("%s:%d", ch, s.Line), []byte(s.Source)})
			}
		}
	}
	return in
}

func TestCorpus_RoundTrip(t *testing.T) {
	inputs := corpusInputs(t)
	var parsed int
	for _, in := range inputs {
		if _, diags := syntax.ParseFile(in.name, in.src); len(diags) == 0 {
			parsed++
		}
	}
	if parsed < 500 {
		t.Fatalf("only %d of %d corpus inputs parse; the corpus went missing", parsed, len(inputs))
	}
	for _, in := range inputs {
		t.Run(in.name, func(t *testing.T) {
			t.Parallel()
			out, diags, err := Source(in.name, in.src)
			if len(diags) > 0 {
				t.Skip("does not parse (a deliberately invalid input or a fragment)")
			}
			if errors.Is(err, ErrUnsafe) {
				t.Fatalf("unsafe: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			again, diags, err := Source(in.name, out)
			if len(diags) > 0 || err != nil {
				t.Fatalf("output does not format again: diags=%v err=%v\n%s", diags, err, out)
			}
			if !bytes.Equal(again, out) {
				t.Errorf("not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
			}
		})
	}
}

func TestSpeed_Examples(t *testing.T) {
	skipTiming(t)
	var srcs []corpusInput
	for _, f := range corpusFiles(t, "../../examples") {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, corpusInput{f, src})
	}
	start := time.Now()
	for _, s := range srcs {
		if _, _, err := Source(s.name, s.src); err != nil {
			t.Fatal(err)
		}
	}
	el := time.Since(start)
	t.Logf("formatted %d files in %v", len(srcs), el)
	if el > 200*time.Millisecond {
		t.Errorf("formatting examples/ took %v, want < 200ms", el)
	}
}

// bigModule builds one module of at least n lines: the first example's header, every example's
// imports (deduplicated) and then the declarations of the examples in turn, repeated as needed.
func bigModule(t testing.TB, n int) []byte {
	t.Helper()
	var bodies []string
	seen := map[string]bool{}
	var imports []string
	for _, f := range corpusFiles(t, "../../examples") {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, diags := syntax.ParseFile(f, src); len(diags) > 0 {
			continue
		}
		var body []string
		for _, l := range strings.Split(string(src), "\n") {
			switch {
			case strings.HasPrefix(l, "module "):
			case strings.HasPrefix(l, "import "):
				if !seen[l] {
					seen[l] = true
					imports = append(imports, l)
				}
			default:
				body = append(body, l)
			}
		}
		bodies = append(bodies, strings.Join(body, "\n"))
	}
	var b strings.Builder
	b.WriteString("module Big\n\n" + strings.Join(imports, "\n") + "\n")
	for lines := 0; lines < n; {
		for _, body := range bodies {
			b.WriteString(body)
			lines += strings.Count(body, "\n")
			if lines >= n {
				break
			}
		}
	}
	return []byte(b.String())
}
