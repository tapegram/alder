package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// timeout bounds every wait of the test client.
const timeout = 5 * time.Second

// client is the test side of a server: it speaks the protocol over io.Pipe to a server running
// in the same process, which tests can also inspect directly (srv).
type client struct {
	t          *testing.T
	w          *io.PipeWriter
	wmu        sync.Mutex // serialises writes to w; held only by send
	mu         sync.Mutex // guards the fields below; never held across a pipe write
	nextID     int
	waits      map[int]chan *message
	notes      chan *message // notifications other than publishDiagnostics
	root       string
	vers       map[string]int    // the version of each open document, by rel
	texts      map[string]string // the text of each open document, by rel
	uriRel     map[string]string // rel by the URI it was opened under
	published  map[string][]publishDiagnosticsParams
	pubSeq     map[string][]int // the sequence number of each of published's entries
	pubs       int              // how many publishDiagnostics have arrived
	mark       int              // pubs when the last document notification was sent
	publishing chan struct{}    // closed (and replaced) at every publishDiagnostics

	srv        *server
	initResult map[string]any
	done       chan struct{} // closed when Serve has returned
	serveErr   error         // what Serve returned; read after done
}

// start runs a server on root over io.Pipe and initializes it (initialize, then initialized).
// The server is stopped (its input closed) when the test ends.
func start(t *testing.T, root string) *client {
	t.Helper()
	return startWith(t, root, Options{})
}

// startWith is start with the server's options (Log is the test log when it is nil).
func startWith(t *testing.T, root string, o Options) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{
		t: t, w: inW, root: root,
		waits: map[int]chan *message{}, notes: make(chan *message, 1024),
		vers: map[string]int{}, texts: map[string]string{}, uriRel: map[string]string{},
		published: map[string][]publishDiagnosticsParams{}, pubSeq: map[string][]int{}, publishing: make(chan struct{}),
		done: make(chan struct{}),
	}
	if o.Log == nil {
		o.Log = testLog{t}
	}
	c.srv = newServer(outW, o)
	go func() {
		c.serveErr = c.srv.serve(inR)
		inR.Close() // later writes fail instead of blocking
		outW.Close()
		close(c.done)
	}()
	go c.read(outR)
	t.Cleanup(func() {
		inW.Close()
		select {
		case <-c.done:
		case <-time.After(timeout):
			t.Error("the server did not stop at EOF")
		}
	})
	if e := c.call("initialize", map[string]any{"rootUri": c.uri(""), "capabilities": map[string]any{}}, &c.initResult); e != nil {
		t.Fatalf("initialize: %v", e)
	}
	c.notify("initialized", map[string]any{})
	return c
}

// testLog sends the server's own messages to the test log.
type testLog struct{ t *testing.T }

func (l testLog) Write(b []byte) (int, error) {
	l.t.Log(strings.TrimRight(string(b), "\n"))
	return len(b), nil
}

// read dispatches the server's messages: responses to their waiter, publishDiagnostics to
// published, other notifications to notes (dropped when nobody reads them).
func (c *client) read(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		m, err := readMessage(br)
		if err != nil {
			return
		}
		switch {
		case m.ID != nil && m.Method == "":
			id, _ := strconv.Atoi(string(*m.ID))
			c.mu.Lock()
			ch := c.waits[id]
			delete(c.waits, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		case m.Method == "textDocument/publishDiagnostics":
			var p publishDiagnosticsParams
			json.Unmarshal(m.Params, &p)
			c.mu.Lock()
			rel := c.relOf(p.URI)
			c.published[rel] = append(c.published[rel], p)
			c.pubs++
			c.pubSeq[rel] = append(c.pubSeq[rel], c.pubs)
			close(c.publishing)
			c.publishing = make(chan struct{})
			c.mu.Unlock()
		case m.ID == nil:
			select {
			case c.notes <- m:
			default:
			}
		}
	}
}

// relOf is the rel of a URI the server sent (c.mu held).
func (c *client) relOf(uri string) string {
	if rel, ok := c.uriRel[uri]; ok {
		return rel
	}
	return strings.TrimPrefix(strings.TrimPrefix(uri, c.uri("")), "/")
}

// send writes one message to the server.
func (c *client) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		c.t.Errorf("marshal: %v", err)
		return
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := io.WriteString(c.w, "Content-Length: "+strconv.Itoa(len(b))+"\r\n\r\n"+string(b)); err != nil {
		c.t.Errorf("write: %v", err)
	}
}

// callAsync sends a request and returns the channel its response arrives on.
func (c *client) callAsync(method string, params any) <-chan *message {
	ch := make(chan *message, 1)
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.waits[id] = ch
	c.mu.Unlock()
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return ch
}

// call sends a request and waits for its response, decoding the result into out (when not nil).
func (c *client) call(method string, params, out any) *rpcError {
	c.t.Helper()
	select {
	case m := <-c.callAsync(method, params):
		if m.Error != nil {
			return m.Error
		}
		if out != nil {
			if err := json.Unmarshal(m.Result, out); err != nil {
				c.t.Fatalf("%s: result %s: %v", method, m.Result, err)
			}
		}
		return nil
	case <-time.After(timeout):
		c.t.Fatalf("%s: no response in %v", method, timeout)
		return nil
	}
}

func (c *client) notify(method string, params any) {
	c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// waitDone waits for Serve to return and returns its error.
func (c *client) waitDone() error {
	c.t.Helper()
	select {
	case <-c.done:
		return c.serveErr
	case <-time.After(timeout):
		c.t.Fatalf("Serve did not return in %v", timeout)
		return nil
	}
}

// uri is the file URI of rel under the root ("" is the root itself).
func (c *client) uri(rel string) string {
	return "file://" + filepath.ToSlash(filepath.Join(c.root, rel))
}

// open sends didOpen for rel, version 1.
func (c *client) open(rel, text string) {
	c.openURI(c.uri(rel), rel, text)
}

// openURI sends didOpen for rel, version 1, under uri (another spelling of its path).
func (c *client) openURI(uri, rel, text string) {
	c.mu.Lock()
	c.vers[rel], c.texts[rel], c.uriRel[uri] = 1, text, rel
	c.mark = c.pubs
	c.mu.Unlock()
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "alder", "version": 1, "text": text}})
}

// change sends didChange for rel with its full new text, at the next version.
func (c *client) change(rel, text string) {
	c.mu.Lock()
	c.vers[rel]++
	c.texts[rel] = text
	v := c.vers[rel]
	uri := c.uri(rel)
	for u, r := range c.uriRel {
		if r == rel {
			uri = u
		}
	}
	c.mark = c.pubs
	c.mu.Unlock()
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": v},
		"contentChanges": []map[string]any{{"text": text}}})
}

// diags waits for the server to publish rel's diagnostics at rel's current version (no version,
// for a document that is not open) and returns the latest such list.
func (c *client) diags(rel string) []Diagnostic {
	c.t.Helper()
	deadline := time.After(timeout)
	for {
		c.mu.Lock()
		want, pubs, signal := c.vers[rel], c.published[rel], c.publishing
		c.mu.Unlock()
		for i := len(pubs) - 1; i >= 0; i-- {
			got := 0
			if pubs[i].Version != nil {
				got = *pubs[i].Version
			}
			if got == want {
				return pubs[i].Diagnostics
			}
		}
		select {
		case <-signal:
		case <-deadline:
			c.t.Fatalf("no diagnostics for %s at version %d in %v", rel, want, timeout)
			return nil
		}
	}
}

// waitPublished waits for the first publish of rel's diagnostics, whatever its version, that
// arrived after the last document notification the client sent, and returns its list.
func (c *client) waitPublished(rel string) []Diagnostic {
	c.t.Helper()
	deadline := time.After(timeout)
	for {
		c.mu.Lock()
		mark, pubs, seqs, signal := c.mark, c.published[rel], c.pubSeq[rel], c.publishing
		c.mu.Unlock()
		for i, seq := range seqs {
			if seq > mark {
				return pubs[i].Diagnostics
			}
		}
		select {
		case <-signal:
		case <-deadline:
			c.t.Fatalf("no diagnostics published for %s in %v", rel, timeout)
			return nil
		}
	}
}

// disk is the text of rel on disk, under the root.
func (c *client) disk(rel string) string {
	c.t.Helper()
	b, err := os.ReadFile(filepath.Join(c.root, rel))
	if err != nil {
		c.t.Fatal(err)
	}
	return string(b)
}

// at is the position of needle's first occurrence in rel's open text, plus off bytes.
func (c *client) at(rel, needle string, off int) Position {
	c.t.Helper()
	c.mu.Lock()
	text, ok := c.texts[rel]
	c.mu.Unlock()
	i := strings.Index(text, needle)
	if !ok || i < 0 {
		c.t.Fatalf("%q is not in the open text of %s", needle, rel)
	}
	i += off
	line := strings.Count(text[:i], "\n")
	return Position{Line: line, Character: i - (strings.LastIndex(text[:i], "\n") + 1)}
}

// blogCopy copies examples/blog, without build/ or databases, to a temporary root.
func blogCopy(t *testing.T) string {
	t.Helper()
	src, root := "../../examples/blog", t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		switch {
		case d.IsDir() && rel == "build":
			return filepath.SkipDir
		case d.IsDir():
			return os.MkdirAll(filepath.Join(root, rel), 0o755)
		case strings.HasSuffix(p, ".db") || !d.Type().IsRegular():
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}
