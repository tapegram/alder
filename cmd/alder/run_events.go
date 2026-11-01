package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"alder/internal/version"
)

// eventRing is how many events the dev server keeps for /_dev/events clients to replay.
const eventRing = 5000

// eventPing is how often an open /_dev/events stream gets a ": ping" comment; the tests shorten it.
var eventPing = 15 * time.Second

// eventHub is the feed behind /_dev/events: every event the dev server emits, as one JSON object
// with its type, its time t (RFC3339Nano, UTC) and its seq (counting from 1 since the dev server
// started). It keeps the last events in a ring for clients to replay, and sends each new one to
// every subscriber. A subscriber too slow to take its events is dropped: its channel is closed,
// so its stream ends and the client resumes with ?after=<the last seq it saw>. Emitting never
// blocks on a subscriber.
type eventHub struct {
	boot string           // the dev server's nonce: a client seeing a new one is talking to another dev server
	now  func() time.Time // the clock of t

	mu   sync.Mutex
	ring [][]byte // the encoded events, oldest first from head once full
	head int      // where the next event goes once the ring is full
	seq  int64    // the last event's seq; 0 before the first
	subs map[chan []byte]struct{}
}

// subBuffer is how many events a subscriber may be behind before it is dropped.
const subBuffer = 256

func newEventHub(boot string, size int) *eventHub {
	return &eventHub{boot: boot, now: time.Now, ring: make([][]byte, 0, size), subs: map[chan []byte]struct{}{}}
}

// encode is the event typ with fields, its time and seq as one JSON object; fields is not changed.
func (h *eventHub) encode(typ string, seq int64, fields map[string]any) []byte {
	m := make(map[string]any, len(fields)+3)
	for k, v := range fields {
		m[k] = v
	}
	m["type"], m["t"], m["seq"] = typ, h.now().UTC().Format(time.RFC3339Nano), seq
	b, err := json.Marshal(m)
	if err != nil { // a field JSON cannot encode: the event still goes out, saying so
		b, _ = json.Marshal(map[string]any{"type": typ, "t": m["t"], "seq": seq, "error": err.Error()})
	}
	return b
}

// emit adds the event typ with fields to the ring and sends it to every subscriber, dropping
// those whose buffer is full. It returns the event's seq.
func (h *eventHub) emit(typ string, fields map[string]any) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	b := h.encode(typ, h.seq, fields)
	if len(h.ring) < cap(h.ring) {
		h.ring = append(h.ring, b)
	} else if len(h.ring) > 0 {
		h.ring[h.head] = b
		h.head = (h.head + 1) % len(h.ring)
	}
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
	return h.seq
}

// subscribe is subscribeAt without the ring's bounds.
func (h *eventHub) subscribe(after int64) (backlog [][]byte, ch <-chan []byte, cancel func()) {
	_, _, backlog, ch, cancel = h.subscribeAt(after)
	return backlog, ch, cancel
}

// subscribeAt subscribes to the events emitted from now on (ch, closed when the subscriber is
// dropped; cancel unsubscribes) and returns, taken at the same instant, the ring's events with a
// seq above after (backlog: the whole ring when after is above the current seq, as the client
// counted on another dev server), the seq of the oldest event in the ring (first, 0 when it is
// empty) and the current seq (last).
func (h *eventHub) subscribeAt(after int64) (first, last int64, backlog [][]byte, ch <-chan []byte, cancel func()) {
	c := make(chan []byte, subBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	n := int64(len(h.ring))
	if n > 0 {
		first = h.seq - n + 1
	}
	k := h.seq - after // how many of the newest events the client lacks
	if after > h.seq || k > n {
		k = n
	}
	backlog = make([][]byte, 0, k)
	for i := n - k; i < n; i++ {
		backlog = append(backlog, h.ring[(int64(h.head)+i)%n])
	}
	h.subs[c] = struct{}{}
	cancel = func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[c]; ok { // not dropped by emit already
			delete(h.subs, c)
			close(c)
		}
	}
	return first, h.seq, backlog, c, cancel
}

// serveEvents is GET /_dev/events[?after=<seq>]: an event stream of the hub's events, each one
// "data: <json>". It starts with a hello (the dev server's app, directory, alder version,
// addresses, generation, build state, start time and boot nonce, the seq of the oldest event
// kept, and as its own seq the current one; hello is not kept in the ring), then replays the
// kept events with a seq above after (all of them without after, or when after is from another
// dev server), then sends each new event, and a ": ping" comment every 15 s. It ends when the
// client goes away, when it falls too far behind (the client resumes with ?after), or when the
// dev server stops.
func (d *devServer) serveEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var after int64
	if s := r.URL.Query().Get("after"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			http.Error(w, "after: not a seq", http.StatusBadRequest)
			return
		}
		after = n
	}
	dir, err := filepath.Abs(d.dir)
	if err != nil {
		dir = filepath.Clean(d.dir)
	}
	d.mu.Lock()
	var child any // null while there is no child
	if d.child != nil {
		child = d.child.addr
	}
	hello := map[string]any{"app": d.app, "dir": dir, "alder": version.String(), "addr": d.addr, "child": child,
		"gen": d.gen, "build": d.state, "since": d.since.UTC().Format(time.RFC3339Nano), "boot": d.hub.boot}
	d.mu.Unlock()
	first, last, backlog, ch, cancel := d.hub.subscribeAt(after)
	defer cancel()
	hello["first"] = first

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	rc := http.NewResponseController(w)
	write := func(b []byte) bool {
		_, err := w.Write(b)
		return err == nil
	}
	data := func(b []byte) bool { return write([]byte("data: ")) && write(b) && write([]byte("\n\n")) }
	if !data(d.hub.encode("hello", last, hello)) {
		return
	}
	for _, b := range backlog {
		if !data(b) {
			return
		}
	}
	if rc.Flush() != nil {
		return
	}
	ping := time.NewTicker(eventPing)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-d.done:
			return
		case b, ok := <-ch:
			if !ok { // dropped: too slow
				return
			}
			if !data(b) || rc.Flush() != nil {
				return
			}
		case <-ping.C:
			if !write([]byte(": ping\n\n")) || rc.Flush() != nil {
				return
			}
		}
	}
}

// serveRestart is POST /_dev/restart: 202, and the worker starts a fresh child from the last
// build (swapIn), or builds the app when no build has succeeded yet. A request with an Origin
// other than the dev server's own address (another site's page) is refused (403), as is any
// other method (405).
func (d *devServer) serveRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if o := r.Header.Get("Origin"); o != "" {
		if u, err := url.Parse(o); err != nil || u.Host != d.addr {
			http.Error(w, "forbidden: cross-origin restart", http.StatusForbidden)
			return
		}
	}
	select {
	case d.restart <- struct{}{}:
	default: // one is already waiting
	}
	w.WriteHeader(http.StatusAccepted)
}
