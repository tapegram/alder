package lsp

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"alder/internal/buildkit"
	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/watch"
)

// analysis is one build of the app with the open documents overlaid: an immutable snapshot
// (Ruling R7), read by any goroutine.
type analysis struct {
	prog     *names.Program
	res      *check.Result // nil after a parse failure
	diags    []diag.Diagnostic
	versions map[string]int      // the doc versions it was built from
	lines    map[string][]string // the text of every module it was built from, by rel
}

// initializedNote starts the analysis goroutine and, when Options.Poll is set, the disk poller,
// whose source changes rebuild and rerun the open scratch documents. Both stop when serve
// returns (at `exit` or the end of input).
func initializedNote(s *server, _ json.RawMessage) {
	if s.started || s.rootReal == "" {
		return
	}
	s.started = true
	s.bg.Go(s.analyse)
	s.schedule() // the first build: closed files' diagnostics, and a last good build before any didOpen
	if s.o.Poll > 0 {
		ch := make(chan watch.Change)
		s.bg.Go(func() { watch.Watch(s.rootReal, s.o.Poll, s.stop, ch) })
		s.bg.Go(func() {
			for {
				select {
				case <-s.stop:
					return
				case k := <-ch:
					if k&watch.SourceChange != 0 {
						s.schedule()
						s.rerunScratch()
					}
				}
			}
		})
	}
}

// schedule asks for a rebuild without waiting: a token in a one-place channel, so any number of
// changes before the analysis goroutine wakes are one rebuild, and a change during a rebuild is
// one more.
func (s *server) schedule() {
	select {
	case s.tokens <- struct{}{}:
	default:
	}
}

// analyse is the analysis goroutine: a rebuild per token, Options.Debounce after it (taking the
// tokens that arrive meanwhile with it).
func (s *server) analyse() {
	for {
		select {
		case <-s.stop:
			return
		case <-s.tokens:
		}
		if s.o.Debounce > 0 {
			select {
			case <-s.stop:
				return
			case <-time.After(s.o.Debounce):
			}
			select {
			case <-s.tokens:
			default:
			}
		}
		s.rebuild()
	}
}

// loadCheckedOverlay is buildkit.LoadCheckedOverlay; a test may replace it before a server starts.
var loadCheckedOverlay = buildkit.LoadCheckedOverlay

// rebuild checks the app with the open documents overlaid, as `alder check` does
// (LoadCheckedOverlay, then AddSchemaDrift: Ruling R13), stores the analysis and publishes its
// diagnostics. A panic in the compiler is logged with its stack and the previous analyses stay,
// so the server keeps serving. Analysis goroutine only.
func (s *server) rebuild() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.o.Log, "alder lsp: rebuild: internal error: %v\n%s", r, debug.Stack())
		}
	}()
	over, vers := s.docs.snapshot()
	prog, res, bag, ioErrs := loadCheckedOverlay(s.rootReal, over)
	if len(ioErrs) > 0 {
		fmt.Fprintf(s.o.Log, "alder lsp: %v\n", ioErrs[0])
		return
	}
	buildkit.AddSchemaDrift(s.rootReal, prog, res, bag)
	a := &analysis{prog: prog, res: res, diags: bag.Items, versions: vers, lines: map[string][]string{}}
	for _, m := range prog.Modules {
		a.lines[m.File] = m.Lines
	}
	for _, m := range prog.Refused {
		a.lines[m.File] = m.Lines
	}
	s.cur.Store(a)
	if res != nil {
		s.good.Store(a)
	}
	s.publish(a)
}

// current is the latest analysis, which has no res after a parse failure; nil before the first.
func (s *server) current() *analysis { return s.cur.Load() }

// lastGood is the latest analysis that type-checked (res != nil); nil before the first.
func (s *server) lastGood() *analysis { return s.good.Load() }

// fresh reports whether a's view of rel still holds at line (1-based, as the compiler counts):
// lines 1..line-1 are unchanged since a was built, and line too when inclusive (Ruling R11).
// Completion asks with inclusive false, since the cursor's line is the one being typed; hover,
// definition and references with true.
func (s *server) fresh(a *analysis, rel string, line int, inclusive bool) bool {
	return freshLines(a, rel, strings.Split(s.text(rel), "\n"), line, inclusive)
}

// freshLines is fresh against now, rel's current lines, which the caller read once.
func freshLines(a *analysis, rel string, now []string, line int, inclusive bool) bool {
	if a == nil {
		return false
	}
	built, ok := a.lines[rel]
	if !ok {
		return false
	}
	last := line - 1
	if inclusive {
		last = line
	}
	if last < 0 || last > len(built) || last > len(now) {
		return false
	}
	for i := 0; i < last; i++ {
		if built[i] != now[i] {
			return false
		}
	}
	return true
}
