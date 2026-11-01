package feed

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
)

// ErrNotAlder is Connect's error when the address answers, but not as an alder dev server: the
// answer is not an event stream, or its first event is not a hello.
var ErrNotAlder = errors.New("not an alder dev server")

// The client's wait before reconnecting starts at backoffMin and doubles after each failed
// attempt, up to backoffMax.
const (
	backoffMin = 250 * time.Millisecond
	backoffMax = 5 * time.Second
)

// Connect opens the feed of the dev server at addr (host:port) and returns its events, starting
// with the hello. The first connection's error is returned as is (ErrNotAlder when something
// other than a dev server answers), so the caller decides whether to attach or start its own.
// Afterwards a dropped stream is reopened with ?after=<the last seq seen>, waiting 250 ms before
// the first attempt and doubling up to 5 s, and the channel carries one Event{Type:
// "disconnected"} per outage, then the new stream's hello. A hello whose Boot differs from the
// previous one (a new dev server, its seq starting again at 1) is passed through as is, followed
// by the new server's whole ring (the client asks again without after when the new server
// replayed only part of it), and the reader resets on it. The channel is closed once ctx ends.
func Connect(ctx context.Context, addr string) (<-chan Event, error) {
	c := &client{url: "http://" + addr + "/_dev/events"}
	s, hello, err := c.open(ctx, -1)
	if err != nil {
		return nil, err
	}
	ch := make(chan Event, 64)
	go c.run(ctx, ch, s, hello)
	return ch, nil
}

// client is one Connect's connection state.
type client struct {
	url string
}

// stream is an open event stream: the response body and its reader.
type stream struct {
	body io.ReadCloser
	r    *bufio.Reader
}

// open requests the feed, with ?after=after unless after is negative, and reads its first event,
// which must be a hello.
func (c *client) open(ctx context.Context, after int64) (*stream, Event, error) {
	u := c.url
	if after >= 0 {
		u += "?after=" + strconv.FormatInt(after, 10)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, Event{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, Event{}, err
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || mt != "text/event-stream" {
		resp.Body.Close()
		return nil, Event{}, ErrNotAlder
	}
	s := &stream{body: resp.Body, r: bufio.NewReader(resp.Body)}
	hello, err := s.next()
	if err != nil || hello.Type != "hello" {
		s.body.Close()
		if ctx.Err() != nil {
			return nil, Event{}, ctx.Err()
		}
		return nil, Event{}, ErrNotAlder
	}
	return s, hello, nil
}

// next is the stream's next event. Comments (": ping") and fields other than data are skipped,
// as is an event whose data does not decode. A line is read whole, however long.
func (s *stream) next() (Event, error) {
	var data []byte
	for {
		line, err := s.r.ReadBytes('\n')
		if err != nil {
			return Event{}, err
		}
		line = bytes.TrimRight(line, "\r\n")
		switch {
		case len(line) == 0: // the end of an event
			if len(data) == 0 {
				continue
			}
			ev, err := Decode(data)
			data = data[:0]
			if err != nil {
				continue
			}
			return ev, nil
		case bytes.HasPrefix(line, []byte("data:")):
			v := bytes.TrimPrefix(line[len("data:"):], []byte(" "))
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, v...)
		}
	}
}

// run sends hello and the events of s to ch, then reopens the feed after each drop, until ctx
// ends; it closes ch.
func (c *client) run(ctx context.Context, ch chan<- Event, s *stream, hello Event) {
	defer close(ch)
	send := func(ev Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	var last int64  // the seq of the last event seen, the hello's aside (it is the server's current seq)
	var boot string // the boot of the dev server last talked to
	for {
		if hello.Hello.Boot != boot { // another dev server: its seqs are not the last one's
			boot, last = hello.Hello.Boot, 0
		}
		if !send(hello) {
			s.body.Close()
			return
		}
		for {
			ev, err := s.next()
			if err != nil {
				break
			}
			if !send(ev) {
				s.body.Close()
				return
			}
			last = ev.Seq
		}
		s.body.Close()
		if ctx.Err() != nil || !send(Event{Type: "disconnected", T: time.Now().UTC()}) {
			return
		}
		var err error
		for wait := backoffMin; ; wait = min(2*wait, backoffMax) {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			if s, hello, err = c.reopen(ctx, last, boot); err == nil {
				break
			}
		}
	}
}

// reopen is open with ?after=last, the seq last seen on the dev server whose boot is boot. When
// another dev server answers (a new alder run, its seq counting again from 1) and has emitted
// last events or more (its hello's seq, its current one, is at least last), it replayed only the
// events above last, so its first ones would be lost: reopen opens the feed again without after,
// for its whole ring.
func (c *client) reopen(ctx context.Context, last int64, boot string) (*stream, Event, error) {
	s, hello, err := c.open(ctx, last)
	if err != nil || hello.Hello.Boot == boot || last == 0 || last > hello.Seq {
		return s, hello, err
	}
	s.body.Close()
	return c.open(ctx, -1)
}
