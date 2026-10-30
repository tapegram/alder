package fill

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The security review of #46: a command cannot outlive its attempt or flood memory, and the API
// path retries an overloaded answer once and quotes no long body.

// TestCommandBackend_TimeoutKillsChildren: when the attempt's time is up, the command's whole
// process group is killed, so a child that would outlive sh does not hold the answer.
func TestCommandBackend_TimeoutKillsChildren(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (&Command{Cmd: "sleep 8; echo x"}).Complete(ctx, "s", []Message{{"user", "x"}})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("returned after %v with %v", time.Since(start), err)
	}
}

// TestCommandBackend_BackgroundChildKilled: a process the command leaves behind, holding its
// standard output, neither holds the answer past the wait delay nor survives it.
func TestCommandBackend_BackgroundChildKilled(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "still-running")
	start := time.Now()
	got, err := (&Command{Cmd: "(sleep 2; touch '" + mark + "') & echo 'Task.succeed Rerender'"}).Complete(context.Background(), "s", []Message{{"user", "x"}})
	if err != nil || strings.TrimSpace(got) != "Task.succeed Rerender" || time.Since(start) > 1900*time.Millisecond {
		t.Fatalf("Complete = %q, %v after %v", got, err, time.Since(start))
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(mark); err == nil {
		t.Error("the background child outlived the attempt")
	}
}

// TestCommandBackend_OutputBounded: standard output past 1 MB fails the attempt.
func TestCommandBackend_OutputBounded(t *testing.T) {
	start := time.Now()
	_, err := (&Command{Cmd: "yes"}).Complete(context.Background(), "s", []Message{{"user", "x"}})
	if err == nil || err.Error() != "ALDER_FILL_COMMAND wrote more than 1 MB: stopped" || time.Since(start) > 5*time.Second {
		t.Errorf("err %v after %v", err, time.Since(start))
	}
}

// TestAnthropic_OverloadedRetriedOnce: a 529 (or 429) is retried once after a short wait; a
// second one is the error, its long body cut.
func TestAnthropic_OverloadedRetriedOnce(t *testing.T) {
	old := overloadedBackoff
	overloadedBackoff = 10 * time.Millisecond
	defer func() { overloadedBackoff = old }()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(529)
			io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
			return
		}
		io.WriteString(w, okAnswer)
	}))
	defer srv.Close()
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_ANTHROPIC_API": srv.URL}))
	if got, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}}); err != nil || got != "Task.succeed Rerender" || calls.Load() != 2 {
		t.Errorf("Complete = %q, %v after %d calls", got, err, calls.Load())
	}
	long := strings.Repeat("x", 5000)
	srv2, reqs, _ := fakeAPI(t, 429, long)
	a = NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "secret-key", "ALDER_ANTHROPIC_API": srv2.URL}))
	_, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}})
	if err == nil || len(*reqs) != 2 || len(err.Error()) > 400 || !strings.HasPrefix(err.Error(), "the Anthropic API answered 429: xxx") || strings.Contains(err.Error(), "secret-key") {
		t.Errorf("err %.80v… (%d bytes) after %d requests", err, len(err.Error()), len(*reqs))
	}
}

// TestSystem_Reference: the system prompt `--prompt` prints carries the reference file first.
func TestSystem_Reference(t *testing.T) {
	ref := filepath.Join(t.TempDir(), "ref.md")
	os.WriteFile(ref, []byte("THE REFERENCE\n"), 0o644)
	if s, err := System(fakeEnv(map[string]string{"ALDER_FILL_REFERENCE": ref})); err != nil || s != "THE REFERENCE\n\n"+Card {
		t.Errorf("System = %.40q, %v", s, err)
	}
	if s, _ := System(fakeEnv(nil)); s != Card {
		t.Error("without a reference the system prompt is not the card")
	}
	if _, err := System(fakeEnv(map[string]string{"ALDER_FILL_REFERENCE": ref + ".missing"})); err == nil {
		t.Error("a missing reference file is not an error")
	}
}

// TestRun_RetryTextNamesTheAnswer: a refusal sent back to the model speaks of its answer, not
// of standard input.
func TestRun_RetryTextNamesTheAnswer(t *testing.T) {
	s := &script{answers: []string{"Task.succeed Rerender,\n}", "Task.succeed Rerender"}}
	if _, err := Run(context.Background(), s, holeApp(t, nil), "Home.update.Submit", "", nil); err != nil {
		t.Fatal(err)
	}
	if retry := s.seen[1][2].Content; strings.Contains(retry, "standard input") || !strings.Contains(retry, "your answer") {
		t.Errorf("retry %q", retry)
	}
}
