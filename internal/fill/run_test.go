package fill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// script is a backend that answers with answers in turn (the last one again after the end) and
// records every conversation it was given.
type script struct {
	mu      sync.Mutex
	answers []string
	seen    [][]Message
}

func (s *script) Complete(ctx context.Context, system string, msgs []Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if system != Card {
		return "", errors.New("the system prompt is not the card")
	}
	s.seen = append(s.seen, append([]Message(nil), msgs...))
	a := s.answers[min(len(s.seen), len(s.answers))-1]
	return a, nil
}

// homeText is app/Home.alder's text in a Result's files ("" when it did not change).
func homeText(r *Result) string {
	for _, f := range r.Files {
		if f.Path == "app/Home.alder" && f.After != nil {
			return *f.After
		}
	}
	return ""
}

// unchanged fails when app/Home.alder differs from before: Run never writes.
func unchanged(t *testing.T, dir, before string) {
	t.Helper()
	if b, _ := os.ReadFile(filepath.Join(dir, "app", "Home.alder")); string(b) != before {
		t.Error("Run wrote to the app")
	}
}

func readHome(t *testing.T, dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "app", "Home.alder"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRun_FirstAttemptApplies(t *testing.T) {
	dir := holeApp(t, nil)
	before := readHome(t, dir)
	s := &script{answers: []string{"```alder\nTask.succeed Rerender\n```\n"}}
	reported := 0
	r, err := Run(context.Background(), s, dir, "Home.update.Submit", "", func(Attempt) { reported++ })
	if err != nil {
		t.Fatal(err)
	}
	if r.Expression != "Task.succeed Rerender" || r.Attempts != 1 || reported != 0 || len(s.seen) != 1 || r.Hole.Name != "done" {
		t.Errorf("result %+v, %d reports, %d calls", r, reported, len(s.seen))
	}
	if got := homeText(r); !strings.Contains(got, "    Task.succeed Rerender\n  },") || strings.Contains(got, "?done") {
		t.Errorf("Home after:\n%s", got)
	}
	if len(r.Edits) != 1 || r.Edits[0].Path != "app/Home.alder" {
		t.Errorf("edits %+v", r.Edits)
	}
	unchanged(t, dir, before)
}

func TestRun_RetriesWithErrors(t *testing.T) {
	dir := holeApp(t, nil)
	s := &script{answers: []string{"Task.sleep 0", "Task.succeed Rerender"}}
	var reports []Attempt
	r, err := Run(context.Background(), s, dir, "Home.update.Submit", "", func(a Attempt) { reports = append(reports, a) })
	if err != nil {
		t.Fatal(err)
	}
	if r.Attempts != 2 || len(reports) != 1 || reports[0].N != 2 || !strings.HasPrefix(reports[0].Why, "E0003 ") || !strings.Contains(reports[0].Why, "Task Unit") {
		t.Errorf("attempts %d, reports %+v", r.Attempts, reports)
	}
	second := s.seen[1]
	if len(second) != 3 || second[1].Role != "assistant" || second[1].Content != "Task.sleep 0" ||
		!strings.HasPrefix(second[2].Content, "The checker refused it:\n") || !strings.Contains(second[2].Content, "error[E0003]") {
		t.Errorf("the second conversation: %+v", second)
	}
}

func TestRun_GivesUpAfterThree(t *testing.T) {
	dir := holeApp(t, nil)
	before := readHome(t, dir)
	s := &script{answers: []string{"42"}}
	_, err := Run(context.Background(), s, dir, "Home.update.Submit", "", nil)
	var f *Failure
	if !errors.As(err, &f) {
		t.Fatalf("err %v", err)
	}
	if f.Expression != "42" || f.Attempts != 3 || len(s.seen) != 3 || f.Why != "the result does not check after 3 attempts: nothing was written" || !strings.Contains(f.Errors, "E0003") {
		t.Errorf("failure %+v, %d calls", f, len(s.seen))
	}
	if txt := f.Text(); !strings.Contains(txt, "the model's last expression:\n42") {
		t.Errorf("text %q", txt)
	}
	unchanged(t, dir, before)
}

func TestRun_NotOneExpressionRetries(t *testing.T) {
	dir := holeApp(t, nil)
	s := &script{answers: []string{"Task.succeed Rerender,\n}\n\nevil = 1", "Task.succeed Rerender"}}
	var reports []Attempt
	r, err := Run(context.Background(), s, dir, "Home.update.Submit", "", func(a Attempt) { reports = append(reports, a) })
	if err != nil {
		t.Fatal(err)
	}
	if r.Attempts != 2 || len(reports) != 1 || !strings.Contains(reports[0].Why, "takes one expression") ||
		!strings.Contains(s.seen[1][2].Content, "takes one expression") {
		t.Errorf("attempts %d, reports %+v, second %+v", r.Attempts, reports, s.seen[1])
	}
}

func TestRun_AddsImport(t *testing.T) {
	dir := holeApp(t, map[string]string{"app/Helpers.alder": "module Helpers\n\ngoHome : Task Nav\ngoHome = Task.succeed (Redirect (Home {}))\n"})
	r, err := Run(context.Background(), &script{answers: []string{"Helpers.goHome"}}, dir, "Home.update.Submit", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Imports) != 1 || r.Imports[0] != "Helpers" || !strings.Contains(homeText(r), "import Helpers\n") {
		t.Errorf("imports %v\n%s", r.Imports, homeText(r))
	}
}

func TestRun_LeavesHoles(t *testing.T) {
	dir := holeApp(t, nil)
	r, err := Run(context.Background(), &script{answers: []string{"Task.succeed ?nav"}}, dir, "Home.update.Submit", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Holes) != 1 || r.Holes[0].Name != "nav" || r.Holes[0].Type != "Nav" {
		t.Errorf("holes %+v", r.Holes)
	}
}

func TestRun_Timeout(t *testing.T) {
	old := AttemptTimeout
	AttemptTimeout = 50 * time.Millisecond
	defer func() { AttemptTimeout = old }()
	dir := holeApp(t, nil)
	block := BackendFunc(func(ctx context.Context, _ string, _ []Message) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	_, err := Run(context.Background(), block, dir, "Home.update.Submit", "", nil)
	var f *Failure
	if !errors.As(err, &f) || f.Why != "no answer in 0.05 s (attempt 1 of 3)" {
		t.Errorf("err %v", err)
	}
}

func TestRun_NoBackend(t *testing.T) {
	if _, err := Run(context.Background(), nil, holeApp(t, nil), "Home.update.Submit", "", nil); !errors.Is(err, NoBackend) {
		t.Errorf("err %v", err)
	}
}
