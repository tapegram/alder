package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testMethod registers a request handler for one test. Call it before start: the registries
// are only read once a server runs, and the handler is removed after the server has stopped.
func testMethod(t *testing.T, reg map[string]handler, name string, h handler) {
	reg[name] = h
	t.Cleanup(func() { delete(reg, name) })
}

// frame is one message as it travels: the Content-Length header, then body.
func frame(body string) string {
	return "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func TestFraming_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	c := &conn{w: &buf}
	c.send(map[string]any{"jsonrpc": "2.0", "id": 1, "result": "é"})
	if !strings.HasPrefix(buf.String(), "Content-Length: ") {
		t.Fatalf("no header: %q", buf.String())
	}
	m, err := readMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	var s string
	json.Unmarshal(m.Result, &s)
	if s != "é" {
		t.Fatalf("result %q", s)
	}
}

func TestFraming_OtherHeadersIgnored(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"exit"}`
	in := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	m, err := readMessage(bufio.NewReader(strings.NewReader(in)))
	if err != nil || m.Method != "exit" {
		t.Fatalf("%+v, %v", m, err)
	}
	if _, err := readMessage(bufio.NewReader(strings.NewReader("Content-Length: x\r\n\r\n"))); err == nil {
		t.Fatal("a bad Content-Length was accepted")
	}
}

func TestInitialize_Capabilities(t *testing.T) {
	c := start(t, blogCopy(t))
	caps := c.initResult["capabilities"].(map[string]any)
	if caps["positionEncoding"] != "utf-8" || caps["hoverProvider"] != true || caps["definitionProvider"] != true || caps["referencesProvider"] != true {
		t.Fatalf("caps %v", caps)
	}
	sync := caps["textDocumentSync"].(map[string]any)
	if sync["change"].(float64) != 1 || sync["openClose"] != true {
		t.Fatalf("sync %v", sync)
	}
	trig := caps["completionProvider"].(map[string]any)["triggerCharacters"].([]any)
	if len(trig) != 3 {
		t.Fatalf("triggers %v", trig)
	}
	cmds := caps["executeCommandProvider"].(map[string]any)["commands"].([]any)
	if len(cmds) != 1 || cmds[0] != "alder.explain" {
		t.Fatalf("commands %v", cmds)
	}
	if info := c.initResult["serverInfo"].(map[string]any); info["name"] != "alder" || info["version"] == "" {
		t.Fatalf("serverInfo %v", info)
	}
	if c.srv.root != c.root || c.srv.rootReal == "" {
		t.Fatalf("root %q, real %q; want %q", c.srv.root, c.srv.rootReal, c.root)
	}
}

func TestUnknownMethod(t *testing.T) {
	c := start(t, blogCopy(t))
	if e := c.call("textDocument/rename", map[string]any{}, nil); e == nil || e.Code != codeMethodNotFound {
		t.Fatalf("err %v", e)
	}
}

func TestPanicIsInternalError(t *testing.T) {
	testMethod(t, fastMethods, "test/panic", func(*server, json.RawMessage) (any, error) { panic("boom") })
	c := start(t, blogCopy(t))
	if e := c.call("test/panic", nil, nil); e == nil || e.Code != codeInternal || e.Message != "internal error: boom" {
		t.Fatalf("err %v", e)
	}
	// the server lives on
	if e := c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Post.alder")}, "position": map[string]any{"line": 0, "character": 0}}, nil); e != nil {
		t.Fatalf("after panic: %v", e)
	}
}

func TestShutdownExit(t *testing.T) {
	c := start(t, blogCopy(t))
	if e := c.call("shutdown", nil, nil); e != nil {
		t.Fatal(e)
	}
	c.notify("exit", nil)
	if err := c.waitDone(); err != nil {
		t.Fatalf("Serve returned %v", err)
	}
}

func TestEOFExits(t *testing.T) {
	c := start(t, blogCopy(t))
	c.w.Close()
	if err := c.waitDone(); err != nil {
		t.Fatalf("Serve returned %v", err)
	}
}

// TestNotInitialized: a request before initialize, and one after shutdown, is refused.
func TestNotInitialized(t *testing.T) {
	in := frame(`{"jsonrpc":"2.0","id":1,"method":"textDocument/hover","params":{}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","id":4,"method":"textDocument/hover","params":{}}`)
	var out bytes.Buffer
	if err := Serve(strings.NewReader(in), &out, Options{}); err != nil {
		t.Fatal(err)
	}
	got := map[string]*message{}
	r := bufio.NewReader(&out)
	for {
		m, err := readMessage(r)
		if err != nil {
			break
		}
		got[string(*m.ID)] = m
	}
	for id, want := range map[string]int{"1": codeNotInitialized, "2": 0, "3": 0, "4": codeNotInitialized} {
		m := got[id]
		switch {
		case m == nil:
			t.Errorf("request %s: no response", id)
		case want == 0 && m.Error != nil:
			t.Errorf("request %s: %v", id, m.Error)
		case want != 0 && (m.Error == nil || m.Error.Code != want):
			t.Errorf("request %s: error %v, want code %d", id, m.Error, want)
		}
	}
}

// TestCancel_StalePreview: while the slow worker is busy, two previews queue up; the first is
// superseded by the second and answered cancelled.
func TestCancel_StalePreview(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	testMethod(t, slowMethods, "test/block", func(*server, json.RawMessage) (any, error) {
		close(started)
		<-release
		return nil, nil
	})
	c := start(t, blogCopy(t))
	blocked := c.callAsync("test/block", nil)
	wait(t, started)
	first := c.callAsync("alder/preview", map[string]any{"id": "new decl"})
	second := c.callAsync("alder/preview", map[string]any{"id": "new decl"})
	c.sync()
	close(release)
	if e := recv(t, first).Error; e == nil || e.Code != codeCancelled {
		t.Fatalf("the superseded preview was answered: %v", e)
	}
	if e := recv(t, second).Error; e != nil {
		t.Fatalf("the latest preview: %v", e)
	}
	recv(t, blocked)
}

// TestCancel_Request: $/cancelRequest for a queued request answers it cancelled.
func TestCancel_Request(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	testMethod(t, slowMethods, "test/block", func(*server, json.RawMessage) (any, error) {
		close(started)
		<-release
		return nil, nil
	})
	testMethod(t, slowMethods, "test/echo", func(*server, json.RawMessage) (any, error) { return "echo", nil })
	c := start(t, blogCopy(t))
	blocked := c.callAsync("test/block", nil)
	wait(t, started)
	cancelled := c.callAsync("test/echo", nil)
	kept := c.callAsync("test/echo", nil)
	c.mu.Lock()
	id := c.nextID - 1
	c.mu.Unlock()
	c.notify("$/cancelRequest", map[string]any{"id": id})
	c.sync()
	close(release)
	if e := recv(t, cancelled).Error; e == nil || e.Code != codeCancelled {
		t.Fatalf("the cancelled request was answered: %v", e)
	}
	if m := recv(t, kept); m.Error != nil || string(m.Result) != `"echo"` {
		t.Fatalf("the other request: %s %v", m.Result, m.Error)
	}
	recv(t, blocked)
}

// TestCancel_IDKinds: request ids are compared as JSON, so the number 1 and the string "1" are
// different requests.
func TestCancel_IDKinds(t *testing.T) {
	testMethod(t, slowMethods, "test/echo", func(*server, json.RawMessage) (any, error) { return "echo", nil })
	var out bytes.Buffer
	s := newServer(&out, Options{})
	s.cancel(json.RawMessage(`1`))
	str, num := json.RawMessage(`"1"`), json.RawMessage(`1`)
	s.handle(&message{ID: &str, Method: "test/echo"})
	s.handle(&message{ID: &num, Method: "test/echo"})
	r := bufio.NewReader(&out)
	for _, want := range []struct {
		id   string
		code int
	}{{`"1"`, 0}, {`1`, codeCancelled}} {
		m, err := readMessage(r)
		if err != nil {
			t.Fatal(err)
		}
		if string(*m.ID) != want.id || (want.code == 0) != (m.Error == nil) || m.Error != nil && m.Error.Code != want.code {
			t.Fatalf("id %s: %+v, want id %s code %d", *m.ID, m, want.id, want.code)
		}
	}
}

// sync returns once the server has read every message sent before it: a fast request, answered
// after the reader has queued everything earlier.
func (c *client) sync() {
	c.t.Helper()
	c.call("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Post.alder")}, "position": map[string]any{"line": 0, "character": 0}}, nil)
}

func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatal("timed out")
	}
}

func recv(t *testing.T, ch <-chan *message) *message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(timeout):
		t.Fatal("no response")
		return nil
	}
}
