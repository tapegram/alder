package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// JSON-RPC error codes, and LSP's.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
	codeCancelled      = -32800
	codeNotInitialized = -32002
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// message is any JSON-RPC message: a request (ID and Method), a notification (Method alone) or
// a response (ID with Result or Error).
type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

// readMessage reads one message: a header block whose Content-Length (the only header it reads)
// gives the length of the JSON body that follows. A body that is not JSON comes back as a
// message carrying a parse error; a broken header is an error, as the stream cannot recover.
func readMessage(r *bufio.Reader) (*message, error) {
	h, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(h.Get("Content-Length"))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("bad Content-Length %q", h.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return &message{Error: &rpcError{codeParse, err.Error()}}, nil
	}
	return &m, nil
}

// idKey is a request id as a map key: its JSON text, compacted, so the number 1 and the
// string "1" stay different ids.
func idKey(id json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, id) != nil {
		return string(id)
	}
	return b.String()
}

// conn writes messages, one at a time, and copies each to wire when it is set.
type conn struct {
	mu   sync.Mutex
	w    io.Writer
	wire io.Writer
}

func (c *conn) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.wire != nil {
		fmt.Fprintf(c.wire, "<- %s\n", b)
	}
	_, err = fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n%s", len(b), b)
	return err
}

// logIn copies a message the server received to wire.
func (c *conn) logIn(m *message) {
	if c.wire == nil {
		return
	}
	b, _ := json.Marshal(m)
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.wire, "-> %s\n", b)
}
