// Package lsp is `alder lsp`: a language server over stdio for editors (alder.nvim). One server
// serves one app root. Its messages are read on one goroutine; requests run on two workers, one
// for fast requests (hover, definition, …) and one for slow ones (dry runs), so a dry run never
// holds up a hover.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"alder/internal/version"
)

type Options struct {
	Log      io.Writer                                // the server's own messages (stderr)
	Wire     io.Writer                                // nil, or every message in and out (ALDER_LSP_LOG)
	Debounce time.Duration                            // rebuild delay after a change; 0 in tests
	Poll     time.Duration                            // disk poll interval; 0 disables
	Migrate  func(dir, slug string) ([]string, error) // generate.App.Migrate
}

// handler answers one request: its result (nil is null), or an error (a *rpcError as is, any
// other as an internal error).
type handler func(s *server, params json.RawMessage) (any, error)

// The registries. Notifications run on the reader goroutine, in order; fast requests run in
// order on one worker, slow requests on another. They are only read once a server runs.
var (
	notifications = map[string]func(*server, json.RawMessage){
		"initialized":            initializedNote,
		"textDocument/didOpen":   didOpen,
		"textDocument/didChange": didChange,
		"textDocument/didClose":  didClose,
	}
	fastMethods = map[string]handler{
		"textDocument/hover":      hoverReq,
		"textDocument/definition": definitionReq,
		"textDocument/references": referencesReq,
		"textDocument/completion": completionReq,
	}
	slowMethods = map[string]handler{
		"alder/preview": func(*server, json.RawMessage) (any, error) { return map[string]any{}, nil },
	}
)

// queueLen is how many requests may wait for each worker before the reader waits too.
const queueLen = 256

type server struct {
	o        Options
	c        *conn
	root     string // the root as the client names it
	rootReal string // root with its symlinks resolved

	fast, slow    chan *message
	cancelled     sync.Map     // idKey → struct{}: requests to answer cancelled
	latestPreview atomic.Value // the idKey of the newest alder/preview

	docs      docs                     // the open documents
	cur, good atomic.Pointer[analysis] // current(), lastGood()
	tokens    chan struct{}            // schedule's: one place, a rebuild pending
	stop      chan struct{}            // closed when serve returns: stops the analysis and the poller
	bg        sync.WaitGroup           // the analysis goroutine and the poller

	// reader goroutine only
	initialized bool
	started     bool // initialized has started the analysis goroutine
	shutdown    bool

	// analysis goroutine only
	reported map[string]bool // the files the last publish gave diagnostics
}

// Serve runs a language server reading requests from r and writing to w. It returns nil at
// `exit` or at the end of r.
func Serve(r io.Reader, w io.Writer, o Options) error {
	return newServer(w, o).serve(r)
}

func newServer(w io.Writer, o Options) *server {
	if o.Log == nil {
		o.Log = io.Discard
	}
	return &server{o: o, c: &conn{w: w, wire: o.Wire},
		fast: make(chan *message, queueLen), slow: make(chan *message, queueLen),
		tokens: make(chan struct{}, 1), stop: make(chan struct{})}
}

// serve is the reader loop. When it returns, the workers have answered every queued request and
// the analysis goroutine and the poller have stopped.
func (s *server) serve(r io.Reader) error {
	defer func() {
		close(s.stop)
		s.bg.Wait()
	}()
	var wg sync.WaitGroup
	for _, ch := range []chan *message{s.fast, s.slow} {
		wg.Go(func() {
			for m := range ch {
				s.handle(m)
			}
		})
	}
	defer func() {
		close(s.fast)
		close(s.slow)
		wg.Wait()
	}()
	br := bufio.NewReader(r)
	for {
		m, err := readMessage(br)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		s.c.logIn(m)
		if s.dispatch(m) {
			return nil
		}
	}
}

// dispatch acts on one message on the reader goroutine, and reports whether it was `exit`.
func (s *server) dispatch(m *message) (exit bool) {
	switch {
	case m.Method == "" && m.JSONRPC == "" && m.Error != nil: // readMessage's parse error
		s.reply(nil, nil, m.Error)
	case m.Method == "" && m.ID != nil && m.Result == nil && m.Error == nil:
		s.reply(m.ID, nil, &rpcError{codeInvalidRequest, "no method"})
	case m.Method == "":
		// a response: the server sends no requests
	case m.ID == nil:
		return s.notification(m)
	case !s.initialized && m.Method != "initialize", s.shutdown:
		s.reply(m.ID, nil, &rpcError{codeNotInitialized, "the server is not initialized"})
	case m.Method == "initialize":
		result, err := s.initialize(m.Params)
		s.reply(m.ID, result, err)
	case m.Method == "shutdown":
		s.shutdown = true
		s.reply(m.ID, nil, nil)
	case fastMethods[m.Method] != nil:
		s.fast <- m
	case slowMethods[m.Method] != nil:
		if m.Method == "alder/preview" {
			s.latestPreview.Store(idKey(*m.ID))
		}
		s.slow <- m
	default:
		s.reply(m.ID, nil, &rpcError{codeMethodNotFound, "unknown method " + m.Method})
	}
	return false
}

// notification runs one notification; ones before initialize, and unknown ones, are ignored.
func (s *server) notification(m *message) (exit bool) {
	switch {
	case m.Method == "exit":
		return true
	case !s.initialized:
	case m.Method == "$/cancelRequest":
		var p cancelParams
		if json.Unmarshal(m.Params, &p) == nil && len(p.ID) > 0 {
			s.cancel(p.ID)
		}
	case notifications[m.Method] != nil:
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(s.o.Log, "alder lsp: %s: internal error: %v\n%s", m.Method, r, debug.Stack())
			}
		}()
		notifications[m.Method](s, m.Params)
	}
	return false
}

// cancel marks the request id to be answered cancelled, unless a worker has started it.
func (s *server) cancel(id json.RawMessage) {
	s.cancelled.Store(idKey(id), struct{}{})
}

// handle answers one request on a worker: cancelled when $/cancelRequest named it, or when it is
// an alder/preview a newer one has superseded; otherwise its handler's answer.
func (s *server) handle(m *message) {
	key := idKey(*m.ID)
	if _, ok := s.cancelled.LoadAndDelete(key); ok {
		s.reply(m.ID, nil, &rpcError{codeCancelled, "cancelled"})
		return
	}
	if m.Method == "alder/preview" && s.latestPreview.Load() != any(key) {
		s.reply(m.ID, nil, &rpcError{codeCancelled, "superseded by a newer preview"})
		return
	}
	h := fastMethods[m.Method]
	if h == nil {
		h = slowMethods[m.Method]
	}
	if h == nil {
		s.reply(m.ID, nil, &rpcError{codeMethodNotFound, "unknown method " + m.Method})
		return
	}
	result, err := s.run(m.Method, h, m.Params)
	s.reply(m.ID, result, err)
}

// run calls h, turning a panic into an internal error (its stack goes to the log).
func (s *server) run(method string, h handler, params json.RawMessage) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.o.Log, "alder lsp: %s: internal error: %v\n%s", method, r, debug.Stack())
			result, err = nil, &rpcError{codeInternal, fmt.Sprintf("internal error: %v", r)}
		}
	}()
	return h(s, params)
}

// reply sends the response to request id (nil: null): err when it is not nil, else result.
func (s *server) reply(id *json.RawMessage, result any, err error) {
	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	if err == nil {
		b, merr := json.Marshal(result)
		if merr != nil {
			err = merr
		} else {
			resp["result"] = json.RawMessage(b)
		}
	}
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = &rpcError{codeInternal, err.Error()}
		}
		resp["error"] = re
	}
	if werr := s.c.send(resp); werr != nil {
		fmt.Fprintf(s.o.Log, "alder lsp: %v\n", werr)
	}
}

// initialize records the root (rootUri, else rootPath) and answers the capabilities.
func (s *server) initialize(params json.RawMessage) (any, error) {
	if s.initialized {
		return nil, &rpcError{codeInvalidRequest, "already initialized"}
	}
	var p initializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
	}
	root := p.RootPath
	if p.RootURI != "" {
		path, err := uriPath(p.RootURI)
		if err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
		root = path
	}
	if root != "" {
		root = filepath.Clean(root)
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, &rpcError{codeInvalidParams, fmt.Sprintf("root: %v", err)}
		}
		s.root, s.rootReal = root, real
	}
	s.initialized = true
	return map[string]any{
		"capabilities": capabilities(),
		"serverInfo":   map[string]any{"name": "alder", "version": version.String()},
	}, nil
}
