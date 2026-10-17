package deploy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const wsTestGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsTestAccept computes the Sec-WebSocket-Accept value for a key.
func wsTestAccept(key string) string {
	h := sha1.Sum([]byte(key + wsTestGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// wsTestFrame builds an unmasked server frame.
func wsTestFrame(fin bool, opcode byte, payload []byte) []byte {
	var b bytes.Buffer
	b0 := opcode
	if fin {
		b0 |= 0x80
	}
	b.WriteByte(b0)
	n := len(payload)
	switch {
	case n < 126:
		b.WriteByte(byte(n))
	case n < 65536:
		b.WriteByte(126)
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(n))
		b.Write(l[:])
	default:
		b.WriteByte(127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		b.Write(l[:])
	}
	b.Write(payload)
	return b.Bytes()
}

// wsTestRead reads one client frame, returning its first byte, mask key and unmasked payload.
func wsTestRead(t *testing.T, r *bufio.Reader) (b0 byte, key [4]byte, payload []byte) {
	t.Helper()
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		t.Errorf("server read header: %v", err)
		return
	}
	b0 = h[0]
	if h[1]&0x80 == 0 {
		t.Errorf("client frame not masked")
	}
	n := int(h[1] & 0x7f)
	if n == 126 {
		var l [2]byte
		io.ReadFull(r, l[:])
		n = int(binary.BigEndian.Uint16(l[:]))
	}
	if _, err := io.ReadFull(r, key[:]); err != nil {
		t.Errorf("server read key: %v", err)
	}
	payload = make([]byte, n)
	io.ReadFull(r, payload)
	for i := range payload {
		payload[i] ^= key[i%4]
	}
	return
}

// wsTestServer starts a server that completes the handshake (unless badAccept)
// and then hands the hijacked connection to fn.
func wsTestServer(t *testing.T, badAccept bool, fn func(conn net.Conn, r *bufio.Reader, req *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		key := req.Header.Get("Sec-WebSocket-Key")
		raw, err := base64.StdEncoding.DecodeString(key)
		if req.Header.Get("Upgrade") != "websocket" || !strings.EqualFold(req.Header.Get("Connection"), "Upgrade") ||
			req.Header.Get("Sec-WebSocket-Version") != "13" || err != nil || len(raw) != 16 {
			t.Errorf("bad handshake headers: %v", req.Header)
		}
		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		acc := wsTestAccept(key)
		if badAccept {
			acc = wsTestAccept("wrong")
		}
		brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + acc + "\r\n\r\n")
		brw.Flush()
		if fn != nil {
			fn(conn, brw.Reader, req)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(srv *httptest.Server) string { return "ws" + strings.TrimPrefix(srv.URL, "http") }

func TestWS_Handshake(t *testing.T) {
	got := make(chan string, 1)
	srv := wsTestServer(t, false, func(conn net.Conn, r *bufio.Reader, req *http.Request) {
		got <- req.Header.Get("Authorization")
	})
	h := http.Header{"Authorization": {"Bearer tok"}}
	c, err := DialWS(context.Background(), wsURL(srv), h)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if a := <-got; a != "Bearer tok" {
		t.Fatalf("Authorization = %q", a)
	}

	bad := wsTestServer(t, true, nil)
	if c, err := DialWS(context.Background(), wsURL(bad), nil); err == nil {
		c.Close()
		t.Fatal("wrong Sec-WebSocket-Accept accepted")
	}
}

func TestWS_TextFrames(t *testing.T) {
	msgs := []string{"hello", strings.Repeat("a", 300), strings.Repeat("b", 70000), "split-message"}
	srv := wsTestServer(t, false, func(conn net.Conn, r *bufio.Reader, req *http.Request) {
		conn.Write(wsTestFrame(true, 1, []byte(msgs[0])))
		conn.Write(wsTestFrame(true, 1, []byte(msgs[1])))
		conn.Write(wsTestFrame(true, 1, []byte(msgs[2])))
		conn.Write(wsTestFrame(false, 1, []byte("split-")))
		conn.Write(wsTestFrame(true, 0, []byte("message")))
		time.Sleep(200 * time.Millisecond)
	})
	c, err := DialWS(context.Background(), wsURL(srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i, want := range msgs {
		got, err := c.Read()
		if err != nil {
			t.Fatalf("msg %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("msg %d: got %d bytes, want %d", i, len(got), len(want))
		}
	}
}

func TestWS_PingPong(t *testing.T) {
	done := make(chan struct{})
	srv := wsTestServer(t, false, func(conn net.Conn, r *bufio.Reader, req *http.Request) {
		defer close(done)
		conn.Write(wsTestFrame(true, 9, []byte("x")))
		b0, _, p := wsTestRead(t, r)
		if b0 != 0x8a || string(p) != "x" {
			t.Errorf("pong: b0=%#x payload=%q", b0, p)
		}
		conn.Write(wsTestFrame(true, 1, []byte("after")))
	})
	c, err := DialWS(context.Background(), wsURL(srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.Read()
	if err != nil || got != "after" {
		t.Fatalf("Read = %q, %v", got, err)
	}
	<-done
}

func TestWS_Close(t *testing.T) {
	done := make(chan struct{})
	srv := wsTestServer(t, false, func(conn net.Conn, r *bufio.Reader, req *http.Request) {
		defer close(done)
		conn.Write(wsTestFrame(true, 8, []byte{0x03, 0xe8}))
		b0, _, p := wsTestRead(t, r)
		if b0 != 0x88 || len(p) < 2 || binary.BigEndian.Uint16(p) != 1000 {
			t.Errorf("close echo: b0=%#x payload=%v", b0, p)
		}
	})
	c, err := DialWS(context.Background(), wsURL(srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Read(); err != io.EOF {
		t.Fatalf("Read err = %v, want io.EOF", err)
	}
	<-done
}

func TestWS_ClientFramesMasked(t *testing.T) {
	got := make(chan [4]byte, 1)
	srv := wsTestServer(t, false, func(conn net.Conn, r *bufio.Reader, req *http.Request) {
		_, key, _ := wsTestRead(t, r) // the client's close frame; wsTestRead fails if unmasked
		got <- key
	})
	c, err := DialWS(context.Background(), wsURL(srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if key := <-got; key == [4]byte{} {
		t.Fatal("mask key is zero")
	}
}
