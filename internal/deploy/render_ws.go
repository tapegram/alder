package deploy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// wsGUID is the RFC 6455 constant mixed into Sec-WebSocket-Accept.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsMaxPayload bounds one frame, and one assembled message.
const wsMaxPayload = 16 << 20

// WSConn is a client websocket connection. Read is for one goroutine; Close
// may be called from any.
type WSConn struct {
	conn net.Conn
	br   *bufio.Reader

	wmu    sync.Mutex // serializes frame writes
	closed bool       // a close frame has been sent
}

// DialWS opens a websocket to a ws:// or wss:// url, sending header on the
// upgrade request. Cancelling ctx closes the connection.
func DialWS(ctx context.Context, rawURL string, header http.Header) (*WSConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}
	var secure bool
	switch u.Scheme {
	case "ws":
	case "wss":
		secure = true
	default:
		return nil, fmt.Errorf("websocket: unsupported scheme %q", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		port := "80"
		if secure {
			port = "443"
		}
		host = net.JoinHostPort(u.Hostname(), port)
	}
	var conn net.Conn
	if secure {
		d := tls.Dialer{Config: &tls.Config{ServerName: u.Hostname()}}
		conn, err = d.DialContext(ctx, "tcp", host)
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, fmt.Errorf("websocket: %w", err)
	}

	// Closing the connection on cancellation unblocks any pending Read.
	stop := context.AfterFunc(ctx, func() { conn.Close() })

	fail := func(err error) (*WSConn, error) {
		stop()
		conn.Close()
		return nil, err
	}

	var keyRaw [16]byte
	if _, err := rand.Read(keyRaw[:]); err != nil {
		return fail(err)
	}
	key := base64.StdEncoding.EncodeToString(keyRaw[:])

	path := u.RequestURI()
	var req bytes.Buffer
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n", path, u.Host, key)
	for k, vs := range header {
		switch http.CanonicalHeaderKey(k) {
		case "Host", "Upgrade", "Connection", "Sec-Websocket-Key", "Sec-Websocket-Version":
			continue
		}
		for _, v := range vs {
			if strings.ContainsAny(k+v, "\r\n") {
				return fail(errors.New("websocket: invalid header"))
			}
			fmt.Fprintf(&req, "%s: %s\r\n", k, v)
		}
	}
	req.WriteString("\r\n")
	if _, err := conn.Write(req.Bytes()); err != nil {
		return fail(fmt.Errorf("websocket: %w", err))
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		return fail(fmt.Errorf("websocket: handshake: %w", err))
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		return fail(fmt.Errorf("websocket: handshake: %s", resp.Status))
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		return fail(errors.New("websocket: handshake: wrong Sec-WebSocket-Accept"))
	}
	return &WSConn{conn: conn, br: br}, nil
}

// Read returns the next text message, joining continuation frames. Pings are
// answered with pongs. A close frame is echoed and reported as io.EOF.
func (c *WSConn) Read() (string, error) {
	var msg []byte
	inMsg := false
	for {
		fin, op, payload, err := c.readFrame()
		if err != nil {
			return "", err
		}
		switch op {
		case 0x9: // ping
			if err := c.writeFrame(0xA, payload); err != nil {
				return "", err
			}
		case 0xA: // pong
		case 0x8: // close
			code := []byte{0x03, 0xe8}
			if len(payload) >= 2 {
				code = payload[:2]
			}
			c.writeClose(code)
			return "", io.EOF
		case 0x1, 0x2, 0x0:
			if op == 0x0 && !inMsg {
				return "", errors.New("websocket: unexpected continuation frame")
			}
			if op != 0x0 && inMsg {
				return "", errors.New("websocket: new message inside a fragmented one")
			}
			inMsg = true
			msg = append(msg, payload...)
			if len(msg) > wsMaxPayload {
				return "", errors.New("websocket: message too large")
			}
			if fin {
				return string(msg), nil
			}
		default:
			return "", fmt.Errorf("websocket: unknown opcode %#x", op)
		}
	}
}

// readFrame reads one frame from the server.
func (c *WSConn) readFrame() (fin bool, op byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(c.br, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	op = h[0] & 0x0f
	if h[1]&0x80 != 0 {
		err = errors.New("websocket: server frame is masked")
		return
	}
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var l [2]byte
		if _, err = io.ReadFull(c.br, l[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(l[:]))
	case 127:
		var l [8]byte
		if _, err = io.ReadFull(c.br, l[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(l[:])
	}
	if n > wsMaxPayload {
		err = errors.New("websocket: frame too large")
		return
	}
	payload = make([]byte, n)
	_, err = io.ReadFull(c.br, payload)
	return
}

// writeFrame sends one masked, final frame.
func (c *WSConn) writeFrame(op byte, payload []byte) error {
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	if mask == [4]byte{} { // the key is specified as non-zero here
		mask[0] = 1
	}
	var b bytes.Buffer
	b.WriteByte(0x80 | op)
	n := len(payload)
	switch {
	case n < 126:
		b.WriteByte(0x80 | byte(n))
	case n < 65536:
		b.WriteByte(0x80 | 126)
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(n))
		b.Write(l[:])
	default:
		b.WriteByte(0x80 | 127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		b.Write(l[:])
	}
	b.Write(mask[:])
	for i, p := range payload {
		b.WriteByte(p ^ mask[i%4])
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.conn.Write(b.Bytes())
	return err
}

// writeClose sends a close frame once.
func (c *WSConn) writeClose(code []byte) error {
	c.wmu.Lock()
	done := c.closed
	c.closed = true
	c.wmu.Unlock()
	if done {
		return nil
	}
	return c.writeFrame(0x8, code)
}

// Close sends close 1000 (if none has been sent) and closes the connection.
func (c *WSConn) Close() error {
	c.writeClose([]byte{0x03, 0xe8})
	return c.conn.Close()
}
