package fill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/syntax"
)

// Attempts is how many answers Run checks before it gives up.
const Attempts = 3

// AttemptTimeout bounds one answer (a variable for tests).
var AttemptTimeout = 120 * time.Second

// Attempt is reported before each attempt after the first: its number and why the one before
// it was refused (the first error, "E0003 expected Task Nav, got Task Unit").
type Attempt struct {
	N   int
	Why string
}

// Result is a fill that checks: the expression, the edits that place it (generate.Fill's) and
// what a dry run of them gives: the files changed, each with its text before and after, the
// imports added and the holes left in the changed files.
type Result struct {
	Expression string
	Hole       generate.Hole
	Edits      []generate.Edit
	Files      []generate.FileChange
	Imports    []string
	Holes      []generate.Hole
	Attempts   int
}

// Failure is a fill that ended without an expression that checks: after the last refusal
// (Errors, as `alder check` prints them), or on a backend error or a timeout (Why). Expression
// is the model's last answer, "" when it gave none, for the user to paste and fix.
type Failure struct {
	Expression string
	Errors     string
	Why        string
	Attempts   int
}

func (f *Failure) Error() string { return f.Why }

// Text is the failure as the editor shows it: why, the errors, and the last expression.
func (f *Failure) Text() string {
	var b strings.Builder
	b.WriteString("alder fill: " + f.Why)
	if f.Errors != "" {
		b.WriteString("\n" + strings.TrimRight(f.Errors, "\n"))
	}
	if f.Expression != "" {
		b.WriteString("\nthe model's last expression:\n" + f.Expression)
	}
	return b.String()
}

// Run fills the hole of the app in dir that address (and hole, "" for the declaration's only
// one) names with backend b: Load, Prompt, then up to Attempts times: Complete (within
// AttemptTimeout), StripFence, generate.Fill, and generate.Apply as a dry run. The first answer
// whose result checks (holes allowed) is the Result; a refused one is retried with the
// checker's errors (a generate.Fill refusal, such as more than one expression, counts as one).
// After the last refusal, on a backend error or a timeout, the error is a *Failure. An app
// that does not load, or an address with no such hole, is generate's refusal. Run never
// writes to dir.
func Run(ctx context.Context, b Backend, dir, address, hole string, report func(Attempt)) (*Result, error) {
	if b == nil {
		return nil, NoBackend
	}
	p, err := generate.Load(dir)
	if err != nil {
		return nil, err
	}
	h, err := generate.FindHole(p, address, hole)
	if err != nil {
		return nil, err
	}
	msgs := []Message{{"user", Prompt(p, *h)}}
	var last, why, errs string
	for n := 1; n <= Attempts; n++ {
		if n > 1 && report != nil {
			report(Attempt{N: n, Why: why})
		}
		actx, cancel := context.WithTimeout(ctx, AttemptTimeout)
		text, err := b.Complete(actx, Card, msgs)
		timedOut := errors.Is(actx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancel()
		if err != nil {
			switch {
			case ctx.Err() != nil:
				return nil, ctx.Err()
			case errors.Is(err, NoBackend):
				return nil, err
			case timedOut:
				why = fmt.Sprintf("no answer in %g s (attempt %d of %d)", AttemptTimeout.Seconds(), n, Attempts)
			default:
				why = fmt.Sprintf("%v (attempt %d of %d)", err, n, Attempts)
			}
			return nil, &Failure{Expression: last, Errors: errs, Why: why, Attempts: n}
		}
		expr := syntax.StripComments(StripFence(text)) // a comment is never placed, on any path
		last = expr
		msgs = append(msgs, Message{"assistant", text})
		edits, _, imports, err := generate.Fill(p, address, hole, expr)
		var ref *generate.Refusal
		if err != nil {
			if !errors.As(err, &ref) {
				return nil, err
			}
			msg := strings.ReplaceAll(ref.Msg, "standard input", "your answer")
			why, errs = msg, msg
			msgs = append(msgs, Message{"user", RetryTurn(errs)})
			continue
		}
		res, err := generate.Apply(dir, edits, generate.Options{DryRun: true})
		if err == nil {
			return &Result{Expression: expr, Hole: *h, Edits: edits, Files: res.Files, Imports: imports, Holes: res.Holes, Attempts: n}, nil
		}
		if !errors.As(err, &ref) {
			return nil, err
		}
		why, errs = refusalText(ref)
		msgs = append(msgs, Message{"user", RetryTurn(errs)})
	}
	return nil, &Failure{Expression: last, Errors: errs, Why: fmt.Sprintf("the result does not check after %d attempts: nothing was written", Attempts), Attempts: Attempts}
}

// refusalText is a refused Apply's first error ("E0003 expected …") and every error as `alder
// check` prints them; the refusal's sentence when it has none.
func refusalText(ref *generate.Refusal) (first, all string) {
	diags, sources := ref.Diags, ref.Sources
	if ref.Result != nil {
		diags, sources = ref.Result.Diagnostics, ref.Result.Sources
	}
	var b strings.Builder
	for _, d := range diags {
		if d.Severity != "error" {
			continue
		}
		if first == "" {
			first = d.Code + " " + d.Message
		}
		b.WriteString(diag.RenderText(d, sources[d.Span.File]))
	}
	if first == "" {
		return ref.Msg, ref.Msg
	}
	return first, b.String()
}
