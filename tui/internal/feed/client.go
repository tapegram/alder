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
	"sync/atomic"
	"time"
)

// ErrNotAlder is Connect's error when the address answers, but not as an alder dev server: the
// answer is not an event stream, or its first event is not a hello.
var ErrNotAlder = errors.New("not an alder dev server")

// errIdle ends a stream that carried no line, not even a ping, for the idle deadline: a
// half-open connection, which the reconnect loop treats like any other drop.
var errIdle = errors.New("feed: no line within the idle deadline")

// defaultIdle is Options.Idle's default, three of the dev server's 15 s pings. headerTimeout is
// how long the client waits for the answer's headers.
const (
	defaultIdle   = 45 * time.Second
	headerTimeout = 10 * time.Second
)

// httpClient is the feed's HTTP client: the default transport, but one that gives up on a dev
// server that accepts the connection and never answers.
var httpClient = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = headerTimeout
	return &http.Client{Transport: t}
}()

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
// replayed only part of it), and the reader resets on it. A reconnect to the same dev server
// whose ring has dropped events the client had not seen is followed by one Event{Type: "gap"}.
// A stream that carries no line for 45 s, pings included, counts as dropped. The channel is
// closed once ctx ends.
func Connect(ctx context.Context, addr string) (<-chan Event, error) {
	return ConnectWith(ctx, addr, Options{})
}

// Options tunes ConnectWith. Idle is how long a stream may carry no line, pings included, before
// the client drops it and reconnects; zero is 45 s, three of the dev server's 15 s pings.
type Options struct {
	Idle time.Duration
}

// ConnectWith is Connect with o.
func ConnectWith(ctx context.Context, addr string, o Options) (<-chan Event, error) {
	if o.Idle <= 0 {
		o.Idle = defaultIdle
	}
	c := &client{url: "http://" + addr + "/_dev/events", idle: o.Idle}
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
	url  string
	idle time.Duration
}

// stream is an open event stream: the response body and its reader, and the idle timer that
// cancels the request once no line has come for idle.
type stream struct {
	body   io.ReadCloser
	r      *bufio.Reader
	idle   time.Duration
	timer  *time.Timer
	idled  atomic.Bool // the timer fired, so the read error is errIdle
	cancel context.CancelFunc
}

// close stops the stream's timer and ends its request.
func (s *stream) close() {
	s.timer.Stop()
	s.cancel()
	s.body.Close()
}

// open requests the feed, with ?after=after unless after is negative, and reads its first event,
// which must be a hello. The hello is under the idle deadline too.
func (c *client) open(ctx context.Context, after int64) (*stream, Event, error) {
	u := c.url
	if after >= 0 {
		u += "?after=" + strconv.FormatInt(after, 10)
	}
	rctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
	if err != nil {
		cancel()
		return nil, Event{}, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, Event{}, err
	}
	s := &stream{body: resp.Body, r: bufio.NewReader(resp.Body), idle: c.idle, cancel: cancel}
	s.timer = time.AfterFunc(c.idle, func() {
		s.idled.Store(true)
		cancel()
	})
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || mt != "text/event-stream" {
		s.close()
		return nil, Event{}, ErrNotAlder
	}
	hello, err := s.next()
	if err != nil || hello.Type != "hello" {
		s.close()
		if ctx.Err() != nil {
			return nil, Event{}, ctx.Err()
		}
		return nil, Event{}, ErrNotAlder
	}
	return s, hello, nil
}

// next is the stream's next event. Comments (": ping") and fields other than data are skipped,
// as is an event whose data does not decode. A line is read whole, however long. Each line,
// a ping's included, pushes the idle deadline back; once it passes, the error is errIdle.
func (s *stream) next() (Event, error) {
	var data []byte
	for {
		line, err := s.r.ReadBytes('\n')
		if err != nil {
			if s.idled.Load() {
				return Event{}, errIdle
			}
			return Event{}, err
		}
		s.timer.Reset(s.idle)
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
	for reconnect := false; ; reconnect = true {
		var missed uint64
		switch {
		case hello.Hello.Boot != boot: // another dev server: its seqs are not the last one's
			boot, last = hello.Hello.Boot, 0
		case reconnect && last > 0 && hello.Hello.First > last+1: // the ring dropped last+1 to First-1
			missed = uint64(hello.Hello.First - last - 1)
		}
		if !send(hello) || missed > 0 && !send(Event{Type: "gap", T: time.Now().UTC(), Gap: &Gap{Missed: missed}}) {
			s.close()
			return
		}
		for {
			ev, err := s.next()
			if err != nil {
				break
			}
			if !send(ev) {
				s.close()
				return
			}
			last = ev.Seq
		}
		s.close()
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
	s.close()
	return c.open(ctx, -1)
}
