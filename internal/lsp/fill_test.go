package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/internal/fill"
)

// The alder.fill tests never call a model: the backend is a Go function.

const fillRel = "app/App.alder"

// fillRoot is the blog with App's Logout arm ending in the hole ?out (Task Nav), and a module
// Helpers whose goLogin fills it (and needs an import).
func fillRoot(t *testing.T) string {
	t.Helper()
	root := blogCopy(t)
	p := filepath.Join(root, "app", "App.alder")
	b, _ := os.ReadFile(p)
	src := strings.Replace(string(b), "    Task.succeed (Redirect (Auth.Login {}))\n", "    ?out\n", 1)
	if src == string(b) {
		t.Fatal("no Logout arm to make a hole of")
	}
	os.WriteFile(p, []byte(src), 0o644)
	os.WriteFile(filepath.Join(root, "app", "Helpers.alder"), []byte("module Helpers\n\ngoLogin : Task Nav\ngoLogin = Task.succeed (Redirect (Auth.Login {}))\n"), 0o644)
	return root
}

var fullCaps = map[string]any{"workspace": map[string]any{"applyEdit": true}, "window": map[string]any{"workDoneProgress": true}}

// answers is a backend answering with each of its answers in turn (the last again after the
// end), recording the conversations; gate, when set, holds every answer until it is closed.
type answers struct {
	mu   sync.Mutex
	list []string
	seen [][]fill.Message
	gate chan struct{}
}

func (a *answers) Complete(ctx context.Context, _ string, msgs []fill.Message) (string, error) {
	if a.gate != nil {
		select {
		case <-a.gate:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seen = append(a.seen, msgs)
	return a.list[min(len(a.seen), len(a.list))-1], nil
}

// fillClient starts a server on root with backend b and the client capabilities caps, and
// opens App.alder as it is on disk.
func fillClient(t *testing.T, root string, b fill.Backend, caps map[string]any) *client {
	t.Helper()
	c := startCaps(t, root, Options{Fill: func() (fill.Backend, string) { return b, "test-model" }}, caps)
	openClean(t, c, fillRel)
	return c
}

func actionsAt(t *testing.T, c *client, rel string, p Position) []action {
	t.Helper()
	var out []action
	if e := c.call("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": c.uri(rel)},
		"range":        Range{Start: p, End: p},
		"context":      map[string]any{"diagnostics": []any{}},
	}, &out); e != nil {
		t.Fatalf("codeAction: %v", e)
	}
	return out
}

// fillAct is the Fill action at ?out, or nil.
func fillAct(t *testing.T, c *client) *action {
	t.Helper()
	for _, a := range actionsAt(t, c, fillRel, c.at(fillRel, "?out", 1)) {
		if a.Command != nil && a.Command.Command == "alder.fill" {
			return &a
		}
	}
	return nil
}

// startFill runs the Fill action's command and returns its answer's started.
func startFill(t *testing.T, c *client) bool {
	t.Helper()
	a := fillAct(t, c)
	if a == nil {
		t.Fatal("no Fill action")
	}
	var res struct {
		Started bool `json:"started"`
	}
	if e := c.call("workspace/executeCommand", map[string]any{"command": "alder.fill", "arguments": []any{a.Command.Arguments[0]}}, &res); e != nil {
		t.Fatalf("executeCommand: %v", e)
	}
	return res.Started
}

type applyEditParams struct {
	Label string `json:"label"`
	Edit  struct {
		DocumentChanges []textDocumentEdit `json:"documentChanges"`
	} `json:"edit"`
}

// applyEdit waits for the server's workspace/applyEdit, answers it with applied, and returns
// its params.
func applyEdit(t *testing.T, c *client, applied bool) applyEditParams {
	t.Helper()
	m := c.request("workspace/applyEdit")
	var p applyEditParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		t.Fatal(err)
	}
	c.answer(m.ID, map[string]any{"applied": applied})
	return p
}

// applyTo applies line edits (whole lines, as the server sends them) to text.
func applyTo(text string, edits []TextEdit) string {
	lines := strings.SplitAfter(text, "\n")
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		lines = append(append(append([]string{}, lines[:e.Range.Start.Line]...), e.NewText), lines[e.Range.End.Line:]...)
	}
	return strings.Join(lines, "")
}

// progressValue is a $/progress notification's value.
func progressValue(t *testing.T, c *client) map[string]any {
	t.Helper()
	var p struct {
		Token any            `json:"token"`
		Value map[string]any `json:"value"`
	}
	json.Unmarshal(c.note("$/progress"), &p)
	return p.Value
}

func showMessage(t *testing.T, c *client) string {
	t.Helper()
	var p struct {
		Type    int    `json:"type"`
		Message string `json:"message"`
	}
	json.Unmarshal(c.note("window/showMessage"), &p)
	if p.Type != 2 {
		t.Errorf("message type %d: %s", p.Type, p.Message)
	}
	return p.Message
}

func TestServer_RequestRoutesResponse(t *testing.T) {
	c := start(t, blogCopy(t))
	type got struct {
		res json.RawMessage
		err error
	}
	done := make(chan got, 2)
	go func() {
		r, err := c.srv.request("test/ping", map[string]any{"x": 1})
		done <- got{r, err}
	}()
	m := c.request("test/ping")
	if string(m.Params) != `{"x":1}` {
		t.Errorf("params %s", m.Params)
	}
	c.send(map[string]any{"jsonrpc": "2.0", "id": "not-a-request-of-the-server", "result": 1}) // ignored
	c.answer(m.ID, map[string]any{"pong": true})
	if g := <-done; g.err != nil || string(g.res) != `{"pong":true}` {
		t.Errorf("request = %s, %v", g.res, g.err)
	}
	go func() {
		r, err := c.srv.request("test/fail", nil)
		done <- got{r, err}
	}()
	m = c.request("test/fail")
	c.answer(m.ID, &rpcError{codeInternal, "no"})
	if g := <-done; g.err == nil || g.err.Error() != "no" {
		t.Errorf("an error answer: %s, %v", g.res, g.err)
	}
}

func TestCodeAction_FillHole(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, fullCaps)
	acts := actionsAt(t, c, fillRel, c.at(fillRel, "?out", 1))
	at := -1
	for i, a := range acts {
		if a.Command != nil && a.Command.Command == "alder.fill" {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no Fill action in %+v", acts)
	}
	a := acts[at]
	if a.Title != "Fill ?out with AI (Task Nav)" || a.Kind != "quickfix" || len(a.Diagnostics) != 1 || a.Diagnostics[0].Code != "W0002" {
		t.Errorf("action %+v", a)
	}
	arg := a.Command.Arguments[0]
	if len(a.Command.Arguments) != 1 || arg["uri"] != c.uri(fillRel) || arg["version"] != float64(1) || arg["address"] != "App.update.Logout" || arg["hole"] != "out" {
		t.Errorf("arguments %+v", a.Command.Arguments)
	}
	for _, b := range acts[:at] {
		if b.Kind == "refactor" {
			t.Errorf("a generator before the Fill action: %s", b.Title)
		}
	}
	var refactors int
	for _, b := range acts[at:] {
		if b.Kind == "refactor" {
			refactors++
		}
	}
	if refactors == 0 {
		t.Error("no generators after the Fill action")
	}
	var caps struct {
		Capabilities struct {
			ExecuteCommandProvider struct{ Commands []string }
		}
	}
	b, _ := json.Marshal(c.initResult)
	json.Unmarshal(b, &caps)
	if !strings.Contains(strings.Join(caps.Capabilities.ExecuteCommandProvider.Commands, ","), "alder.fill") {
		t.Errorf("executeCommandProvider %v", caps.Capabilities.ExecuteCommandProvider.Commands)
	}
	// not on a line without a hole
	for _, a := range actionsAt(t, c, fillRel, c.at(fillRel, "login = Auth.Login", 0)) {
		if a.Command != nil && a.Command.Command == "alder.fill" {
			t.Errorf("a Fill action off the hole: %+v", a)
		}
	}
}

func TestCodeAction_FillHoleWithoutApplyEdit(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, map[string]any{})
	if a := fillAct(t, c); a != nil {
		t.Errorf("a Fill action for a client without workspace.applyEdit: %+v", a)
	}
}

func TestCodeAction_FillHoleStaleLine(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, fullCaps)
	pos := c.at(fillRel, "?out", 1)
	c.change(fillRel, strings.Replace(c.disk(fillRel), "    ?out\n", "    ?out (\n", 1))
	c.diags(fillRel)
	for _, a := range actionsAt(t, c, fillRel, pos) {
		if a.Command != nil && a.Command.Command == "alder.fill" {
			t.Errorf("a Fill action on a changed line: %+v", a)
		}
	}
}

func TestFill_AppliesVersionedEdit(t *testing.T) {
	b := &answers{list: []string{"Helpers.goLogin"}}
	c := fillClient(t, fillRoot(t), b, fullCaps)
	if !startFill(t, c) {
		t.Fatal("not started")
	}
	m := c.request("window/workDoneProgress/create")
	c.answer(m.ID, nil)
	if v := progressValue(t, c); v["kind"] != "begin" || v["title"] != "alder: filling ?out : Task Nav" || v["message"] != "test-model" {
		t.Errorf("begin %v", v)
	}
	p := applyEdit(t, c, true)
	if v := progressValue(t, c); v["kind"] != "end" {
		t.Errorf("end %v", v)
	}
	if p.Label != "alder fill ?out" || len(p.Edit.DocumentChanges) != 1 {
		t.Fatalf("applyEdit %+v", p)
	}
	dc := p.Edit.DocumentChanges[0]
	if dc.TextDocument.URI != c.uri(fillRel) || dc.TextDocument.Version == nil || *dc.TextDocument.Version != 1 {
		t.Errorf("document %+v", dc.TextDocument)
	}
	if len(dc.Edits) != 2 {
		t.Fatalf("want two edits (the import, the hole), got %+v", dc.Edits)
	}
	after := applyTo(c.disk(fillRel), dc.Edits)
	if !strings.Contains(after, "import Helpers\n") || !strings.Contains(after, "    Helpers.goLogin\n") || strings.Contains(after, "?out") {
		t.Errorf("applied:\n%s", after)
	}
	if strings.Contains(c.disk(fillRel), "Helpers.goLogin") {
		t.Error("the server wrote the file")
	}
}

func TestFill_RetryThenApply(t *testing.T) {
	b := &answers{list: []string{"42", "Task.succeed (Redirect (Auth.Login {}))"}}
	c := fillClient(t, fillRoot(t), b, fullCaps)
	startFill(t, c)
	c.answer(c.request("window/workDoneProgress/create").ID, nil)
	progressValue(t, c) // begin
	if v := progressValue(t, c); v["kind"] != "report" || !strings.HasPrefix(v["message"].(string), "attempt 2 of 3: E0003 ") {
		t.Errorf("report %v", v)
	}
	p := applyEdit(t, c, true)
	if after := applyTo(c.disk(fillRel), p.Edit.DocumentChanges[0].Edits); !strings.Contains(after, "    Task.succeed (Redirect (Auth.Login {}))\n  },") {
		t.Errorf("applied:\n%s", after)
	}
	if len(b.seen) != 2 || !strings.Contains(b.seen[1][2].Content, "E0003") {
		t.Errorf("the retry: %+v", b.seen)
	}
}

func TestFill_GivesUpShowsMessage(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"42"}}, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	startFill(t, c)
	msg := showMessage(t, c)
	if !strings.HasPrefix(msg, "alder fill: the result does not check after 3 attempts: nothing was written\n") ||
		!strings.Contains(msg, "E0003") || !strings.HasSuffix(msg, "the model's last expression:\n42") {
		t.Errorf("message %q", msg)
	}
	select {
	case m := <-c.requests:
		t.Errorf("a request after a failure: %s", m.Method)
	default:
	}
}

func TestFill_ClientRefusesEdit(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"Task.succeed (Redirect (Auth.Login {}))"}}, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	startFill(t, c)
	applyEdit(t, c, false)
	if msg := showMessage(t, c); msg != "alder fill: app/App.alder changed while filling; the expression was:\nTask.succeed (Redirect (Auth.Login {}))" {
		t.Errorf("message %q", msg)
	}
}

func TestFill_OneAtATime(t *testing.T) {
	b := &answers{list: []string{"Task.succeed (Redirect (Auth.Login {}))"}, gate: make(chan struct{})}
	c := fillClient(t, fillRoot(t), b, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	if !startFill(t, c) {
		t.Fatal("the first fill did not start")
	}
	if startFill(t, c) {
		t.Fatal("a second fill started while the first runs")
	}
	if msg := showMessage(t, c); msg != "alder fill: a fill is already running; wait for it to end" {
		t.Errorf("message %q", msg)
	}
	close(b.gate)
	applyEdit(t, c, true)
	// once it ends, another may start
	deadline := time.Now().Add(timeout)
	for c.srv.filling.Load() {
		if time.Now().After(deadline) {
			t.Fatal("the fill did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFill_NoBackendMessage(t *testing.T) {
	c := fillClient(t, fillRoot(t), nil, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	if fillAct(t, c) == nil {
		t.Fatal("no Fill action without a backend: it should be offered, so it can be found")
	}
	if startFill(t, c) {
		t.Error("started without a backend")
	}
	if msg := showMessage(t, c); msg != "alder fill: set ANTHROPIC_API_KEY, or ALDER_FILL_COMMAND, in the environment the editor was started from" {
		t.Errorf("message %q", msg)
	}
}

func TestFill_UsesUnsavedBuffer(t *testing.T) {
	b := &answers{list: []string{"Task.succeed (Redirect (Auth.Login {}))"}}
	c := fillClient(t, fillRoot(t), b, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	text := strings.Replace(c.disk(fillRel), "login = Auth.Login\n", "login = Auth.Login\n\n-- an unsaved note\n", 1)
	c.change(fillRel, text)
	c.diags(fillRel)
	startFill(t, c)
	p := applyEdit(t, c, true)
	dc := p.Edit.DocumentChanges[0]
	if dc.TextDocument.Version == nil || *dc.TextDocument.Version != 2 {
		t.Errorf("version %v", dc.TextDocument.Version)
	}
	if after := applyTo(text, dc.Edits); !strings.Contains(after, "-- an unsaved note\n") || !strings.Contains(after, "    Task.succeed (Redirect (Auth.Login {}))\n  },") {
		t.Errorf("applied to the buffer:\n%s", after)
	}
	if !strings.Contains(b.seen[0][0].Content, "-- an unsaved note") {
		t.Error("the prompt is not the buffer's")
	}
}

func TestFill_NoProgressCapability(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"Task.succeed (Redirect (Auth.Login {}))"}}, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	startFill(t, c)
	select {
	case m := <-c.requests:
		if m.Method != "workspace/applyEdit" {
			t.Fatalf("first request %s", m.Method)
		}
		c.answer(m.ID, map[string]any{"applied": true})
	case <-time.After(noteTimeout):
		t.Fatal("no applyEdit")
	}
	for {
		select {
		case m := <-c.notes:
			if m.Method == "$/progress" {
				t.Errorf("progress without the capability: %s", m.Params)
			}
			continue
		default:
		}
		break
	}
}
