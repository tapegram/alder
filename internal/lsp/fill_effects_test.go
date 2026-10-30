package lsp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The final security review of #46: what a model wrote never lands unseen, and an expression
// with effects its hole's type does not limit is applied only when the user says so.

// infoMessage waits for the next window/showMessage and returns its type and text.
func infoMessage(t *testing.T, c *client) (int, string) {
	t.Helper()
	var p struct {
		Type    int    `json:"type"`
		Message string `json:"message"`
	}
	json.Unmarshal(c.note("window/showMessage"), &p)
	return p.Type, p.Message
}

const sessionPut = "{\n  let _ <- Session.put (\\s -> s)\n  Task.succeed (Redirect (Auth.Login {}))\n}"

var askCaps = map[string]any{"workspace": map[string]any{"applyEdit": true}, "window": map[string]any{"showMessage": map[string]any{}}}

func TestFill_ShowsAppliedExpression(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"Task.succeed (Redirect (Auth.Login {}))"}}, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	startFill(t, c)
	applyEdit(t, c, true)
	if typ, msg := infoMessage(t, c); typ != 3 || msg != "alder fill ?out: Task.succeed (Redirect (Auth.Login {}))" {
		t.Errorf("message %d %q", typ, msg)
	}
}

func TestFill_EffectsAskBeforeApply(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{sessionPut}}, askCaps)
	startFill(t, c)
	m := c.request("window/showMessageRequest")
	var q struct {
		Type    int
		Message string
		Actions []struct{ Title string }
	}
	json.Unmarshal(m.Params, &q)
	if q.Type != 2 || !strings.HasPrefix(q.Message, "alder fill ?out: the model's expression uses Session.put, effects its hole's type does not limit: review it. Apply it?\n{") ||
		len(q.Actions) != 2 || q.Actions[0].Title != "Apply" || q.Actions[1].Title != "Discard" {
		t.Fatalf("question %+v", q)
	}
	c.answer(m.ID, map[string]any{"title": "Apply"})
	p := applyEdit(t, c, true)
	if after := applyTo(c.disk(fillRel), p.Edit.DocumentChanges[0].Edits); !strings.Contains(after, "Session.put (\\s -> s)") {
		t.Errorf("applied:\n%s", after)
	}
	if typ, msg := infoMessage(t, c); typ != 3 || !strings.HasPrefix(msg, "alder fill ?out: {\n") {
		t.Errorf("message %d %q", typ, msg)
	}
}

func TestFill_EffectsDiscarded(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{sessionPut}}, askCaps)
	startFill(t, c)
	m := c.request("window/showMessageRequest")
	c.answer(m.ID, map[string]any{"title": "Discard"})
	if typ, msg := infoMessage(t, c); typ != 3 || !strings.HasPrefix(msg, "alder fill ?out: discarded; the expression was:\n{") {
		t.Errorf("message %d %q", typ, msg)
	}
	noRequest(t, c)
}

func TestFill_EffectsWithoutAskCapability(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{sessionPut}}, map[string]any{"workspace": map[string]any{"applyEdit": true}})
	startFill(t, c)
	if typ, msg := infoMessage(t, c); typ != 2 || !strings.Contains(msg, "uses Session.put") || !strings.Contains(msg, "Not applied (the editor cannot ask); the expression:\n{") {
		t.Errorf("message %d %q", typ, msg)
	}
	noRequest(t, c)
}

// noRequest fails when the server sends a request within a short while.
func noRequest(t *testing.T, c *client) {
	t.Helper()
	select {
	case m := <-c.requests:
		t.Errorf("a request: %s", m.Method)
	case <-time.After(300 * time.Millisecond):
	}
}
