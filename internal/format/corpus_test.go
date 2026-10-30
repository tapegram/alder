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

// speedTestsEnv opts in to the wall-clock tests.
const speedTestsEnv = "ALDER_SPEED_TESTS"

// skipTiming skips a wall-clock test unless ALDER_SPEED_TESTS=1, and always under -short and
// -race. Wall-clock bounds flake on a loaded machine (TestSpeed_Examples took 262-756 ms against
// its 200 ms bound on a busy laptop while the formatter had not changed), so they are a check run
// by hand on a quiet machine. The always-on guard against a slower algorithm is the allocation
// scaling tests (TestScaling_*), which do not depend on the machine.
func skipTiming(t *testing.T) {
	t.Helper()
	if testing.Short() || raceEnabled || os.Getenv(speedTestsEnv) != "1" {
		t.Skip("wall-clock test: run with " + speedTestsEnv + "=1 on a quiet machine (never under -short or -race)")
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
	// 920 of 934 parsed on 2026-10-30; the floor sits a little under that, so losing a directory
	// or the book's snippets fails the test while adding a deliberately invalid input does not
	if parsed < 900 {
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
		decls := false // the body holds more than blank lines
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
				decls = decls || strings.TrimSpace(l) != ""
			}
		}
		if decls {
			bodies = append(bodies, strings.Join(body, "\n"))
		}
	}
	if len(bodies) == 0 { // the loop below would never reach n lines
		t.Fatal("bigModule: no example has declarations")
	}
	var b strings.Builder
	b.WriteString("module Big\n\n" + strings.Join(imports, "\n") + "\n")
	for lines := 0; lines < n; {
		for _, body := range bodies {
			b.WriteString(body + "\n") // a newline ends each, so every body counts at least one line
			lines += strings.Count(body, "\n") + 1
			if lines >= n {
				break
			}
		}
	}
	return []byte(b.String())
}

// TestScaling_Source guards against a formatter that does more than linear work, whatever the
// machine's load: formatting a module of at least 4000 lines rather than 1000 (about three times
// the bytes) allocates at most 1.5 times more per byte. (Allocation counts are deterministic;
// wall-clock time is the opt-in TestSpeed_Examples.) The scaling tests must not call t.Parallel:
// AllocsPerRun counts every allocation in the process, so a parallel test's would be counted too.
func TestScaling_Source(t *testing.T) {
	if testing.Short() {
		t.Skip("formats a 4000-line module")
	}
	small, big := bigModule(t, 1000), bigModule(t, 4000)
	allocs := func(src []byte) float64 {
		return testing.AllocsPerRun(1, func() {
			if _, diags, err := Source("Big.alder", src); err != nil || len(diags) > 0 {
				t.Fatalf("generated module does not format: diags=%v err=%v", diags, err)
			}
		})
	}
	as, ab := allocs(small), allocs(big)
	perByte := (ab / float64(len(big))) / (as / float64(len(small)))
	t.Logf("%d bytes: %.0f allocs; %d bytes: %.0f allocs; per-byte ratio %.2f", len(small), as, len(big), ab, perByte)
	if perByte > 1.5 {
		t.Errorf("allocations per byte grew %.2fx from a %d- to a %d-byte module, want at most 1.5x", perByte, len(small), len(big))
	}
}

// deepModule nests depth levels of one shape whose bodies are hugged calls, each level going
// through the printer's body layout (after) again: `let` statements calling with a record, match
// arms calling with a lambda, and lambdas inside hugged lambdas.
func deepModule(depth int, shape string) []byte {
	body := "g"
	for i := 0; i < depth; i++ {
		switch shape {
		case "let":
			body = fmt.Sprintf("{\nlet a%d = f%d q { k: %s }\na%d\n}", i, i, body, i)
		case "arm":
			body = fmt.Sprintf("match x%d {\n_ -> f%d a (\\y -> %s),\n}", i, i, body)
		case "lambda":
			body = fmt.Sprintf("f%d q (\\y%d -> h%d r (\\z%d -> %s))", i, i, i, i, body)
		}
	}
	return []byte("module Deep\n\nv = " + body + "\n")
}

// TestScaling_Depth guards against work that multiplies with nesting: a layout choice that built
// its body once per alternative doubled the time at every level (depth 24 took over a minute).
// Three times the depth must allocate at most twice as much per byte of input (doubling per level
// would be 4096 times). The depths stay small so that such a regression fails fast instead of
// hanging the test.
func TestScaling_Depth(t *testing.T) {
	for _, shape := range []string{"let", "arm", "lambda"} {
		allocs := func(src []byte) float64 {
			return testing.AllocsPerRun(1, func() {
				if _, diags, err := Source("Deep.alder", src); err != nil || len(diags) > 0 {
					t.Fatalf("%s: does not format: diags=%v err=%v", shape, diags, err)
				}
			})
		}
		small, big := deepModule(6, shape), deepModule(18, shape)
		as, ab := allocs(small), allocs(big)
		perByte := (ab / float64(len(big))) / (as / float64(len(small)))
		t.Logf("%s: depth 6 %.0f allocs, depth 18 %.0f allocs, per-byte ratio %.2f", shape, as, ab, perByte)
		if perByte > 2 {
			t.Errorf("%s: allocations per byte grew %.2fx from depth 6 to 18, want at most 2x", shape, perByte)
		}
	}
}
