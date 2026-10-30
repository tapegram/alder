package fill

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAPI is an httptest server standing in for the Messages API: it records each request and
// answers with status and body.
func fakeAPI(t *testing.T, status int, body string) (*httptest.Server, *[]*http.Request, *[][]byte) {
	t.Helper()
	var reqs []*http.Request
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		reqs, bodies = append(reqs, r), append(bodies, b)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs, &bodies
}

const okAnswer = `{"content":[{"type":"text","text":"Task.succeed Rerender"}],"stop_reason":"end_turn"}`

// fakeEnv is a getenv over m: never the process's environment, so no test reads a real key.
func fakeEnv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestAnthropic_RequestShape(t *testing.T) {
	srv, reqs, bodies := fakeAPI(t, 200, okAnswer)
	ref := filepath.Join(t.TempDir(), "reference.md")
	os.WriteFile(ref, []byte("THE REFERENCE"), 0o644)
	b, model := FromEnv(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "test-key", "ALDER_ANTHROPIC_API": srv.URL + "/", "ALDER_FILL_REFERENCE": ref}))
	if model != DefaultModel {
		t.Errorf("model %q", model)
	}
	msgs := []Message{{"user", "fill it"}, {"assistant", "42"}, {"user", "again"}}
	got, err := b.Complete(context.Background(), "THE CARD", msgs)
	if err != nil || got != "Task.succeed Rerender" {
		t.Fatalf("Complete = %q, %v", got, err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("%d requests", len(*reqs))
	}
	r := (*reqs)[0]
	if r.Method != "POST" || r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("request %s %s, headers %v", r.Method, r.URL.Path, r.Header)
	}
	var body map[string]any
	if err := json.Unmarshal((*bodies)[0], &body); err != nil {
		t.Fatal(err)
	}
	want := `{"max_tokens":4096,"messages":[{"content":"fill it","role":"user"},{"content":"42","role":"assistant"},{"content":"again","role":"user"}],"model":"claude-opus-5-5","system":[{"cache_control":{"type":"ephemeral"},"text":"THE REFERENCE","type":"text"},{"cache_control":{"type":"ephemeral"},"text":"THE CARD","type":"text"}]}`
	if g, _ := json.Marshal(body); string(g) != want {
		t.Errorf("body\n%s\nwant\n%s", g, want)
	}
	if strings.Contains(string((*bodies)[0]), "thinking") || strings.Contains(string((*bodies)[0]), "effort") {
		t.Error("the request sets thinking or effort")
	}
	// without a reference: one system block; ALDER_FILL_MODEL picks the model
	b, model = FromEnv(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_ANTHROPIC_API": srv.URL, "ALDER_FILL_MODEL": "claude-haiku-5-5"}))
	if _, err := b.Complete(context.Background(), "C", msgs[:1]); err != nil || model != "claude-haiku-5-5" {
		t.Fatal(err, model)
	}
	json.Unmarshal((*bodies)[1], &body)
	if sys := body["system"].([]any); len(sys) != 1 || body["model"] != "claude-haiku-5-5" {
		t.Errorf("system %v, model %v", sys, body["model"])
	}
	// the key is in no String
	if s := b.(*Anthropic).String(); strings.Contains(s, "k}") || strings.Contains(s, "Key") {
		t.Errorf("String shows the key: %s", s)
	}
}

func TestAnthropic_NoKey(t *testing.T) {
	if b, _ := FromEnv(fakeEnv(nil)); b != nil {
		t.Errorf("no key, no command: backend %v", b)
	}
	a := NewAnthropic(fakeEnv(map[string]string{"ALDER_ANTHROPIC_API": "http://127.0.0.1:1"}))
	if _, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}}); !errors.Is(err, NoBackend) {
		t.Errorf("err %v", err)
	}
	if !strings.Contains(NoBackend.Error(), "ANTHROPIC_API_KEY") || !strings.Contains(NoBackend.Error(), "ALDER_FILL_COMMAND") {
		t.Errorf("message %q", NoBackend)
	}
	// the command wins over a key
	if b, model := FromEnv(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_FILL_COMMAND": "cat"})); model != "command" || b.(*Command).Cmd != "cat" {
		t.Errorf("command backend: %v %q", b, model)
	}
}

func TestAnthropic_HTTPError(t *testing.T) {
	srv, _, _ := fakeAPI(t, 529, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "secret-key", "ALDER_ANTHROPIC_API": srv.URL}))
	_, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}})
	if err == nil || err.Error() != "the Anthropic API answered 529: Overloaded" {
		t.Errorf("err %v", err)
	}
	var nr *NoRetry
	if errors.As(err, &nr) {
		t.Error("an HTTP error is NoRetry")
	}
}

func TestAnthropic_RefusalNoRetry(t *testing.T) {
	srv, _, _ := fakeAPI(t, 200, `{"content":[],"stop_reason":"refusal"}`)
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_ANTHROPIC_API": srv.URL}))
	_, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}})
	var nr *NoRetry
	if !errors.As(err, &nr) || nr.Msg != "the model declined" {
		t.Errorf("err %v", err)
	}
}

func TestAnthropic_MaxTokensNoRetry(t *testing.T) {
	srv, _, _ := fakeAPI(t, 200, `{"content":[{"type":"text","text":"Task.suc"}],"stop_reason":"max_tokens"}`)
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "k", "ALDER_ANTHROPIC_API": srv.URL}))
	_, err := a.Complete(context.Background(), "s", []Message{{"user", "x"}})
	var nr *NoRetry
	if !errors.As(err, &nr) || nr.Msg != "the answer did not end" {
		t.Errorf("err %v", err)
	}
}

// TestAnthropic_LogHasNoKey: the wire log (ALDER_LSP_LOG) gets the body and the answer, never
// the key.
func TestAnthropic_LogHasNoKey(t *testing.T) {
	srv, _, _ := fakeAPI(t, 200, okAnswer)
	a := NewAnthropic(fakeEnv(map[string]string{"ANTHROPIC_API_KEY": "secret-key", "ALDER_ANTHROPIC_API": srv.URL}))
	var log strings.Builder
	a.Log = &log
	if _, err := a.Complete(context.Background(), "THE CARD", []Message{{"user", "x"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "THE CARD") || !strings.Contains(log.String(), "Task.succeed Rerender") || strings.Contains(log.String(), "secret-key") {
		t.Errorf("log:\n%s", log.String())
	}
}

func TestCommandBackend_StdinIsPromptStdoutIsExpression(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "stdin")
	c := &Command{Cmd: "cat > " + in + "; printf 'Task.succeed Rerender\\n'"}
	got, err := c.Complete(context.Background(), "THE CARD", []Message{{"user", "fill it"}, {"assistant", "42"}, {"user", "again"}})
	if err != nil || got != "Task.succeed Rerender\n" {
		t.Fatalf("Complete = %q, %v", got, err)
	}
	b, _ := os.ReadFile(in)
	if want := "THE CARD\n\nfill it\n\nYour answer:\n42\n\nagain\n"; string(b) != want {
		t.Errorf("stdin %q, want %q", b, want)
	}
}

func TestCommandBackend_NonZeroExitFails(t *testing.T) {
	c := &Command{Cmd: "echo 'no model here' >&2; exit 3"}
	_, err := c.Complete(context.Background(), "s", []Message{{"user", "x"}})
	if err == nil || err.Error() != "ALDER_FILL_COMMAND failed: no model here" {
		t.Errorf("err %v", err)
	}
}
