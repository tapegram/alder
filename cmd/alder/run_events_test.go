package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// backlogSeqs is the seq of each event in backlog, which must be JSON objects.
func backlogSeqs(t *testing.T, backlog [][]byte) []int64 {
	t.Helper()
	var seqs []int64
	for _, b := range backlog {
		var e struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatalf("event %q: %v", b, err)
		}
		seqs = append(seqs, e.Seq)
	}
	return seqs
}

func emitN(h *eventHub, n int) {
	for i := 0; i < n; i++ {
		h.emit("run", map[string]any{"msg": "event " + strconv.Itoa(i+1)})
	}
}

func TestEventHub_SeqAndRing(t *testing.T) {
	h := newEventHub("b00t", 3)
	emitN(h, 5)
	backlog, _, cancel := h.subscribe(0)
	defer cancel()
	if got := backlogSeqs(t, backlog); !slices.Equal(got, []int64{3, 4, 5}) {
		t.Errorf("backlog seqs = %v, want [3 4 5]", got)
	}
	var e map[string]any
	if err := json.Unmarshal(backlog[2], &e); err != nil {
		t.Fatal(err)
	}
	if e["type"] != "run" || e["msg"] != "event 5" {
		t.Errorf("event = %v", e)
	}
	if _, err := time.Parse(time.RFC3339Nano, e["t"].(string)); err != nil || !strings.HasSuffix(e["t"].(string), "Z") {
		t.Errorf("t = %v, want RFC3339Nano in UTC (%v)", e["t"], err)
	}
}

func TestEventHub_After(t *testing.T) {
	h := newEventHub("b00t", 10)
	emitN(h, 5)
	backlog, _, cancel := h.subscribe(3)
	defer cancel()
	if got := backlogSeqs(t, backlog); !slices.Equal(got, []int64{4, 5}) {
		t.Errorf("backlog seqs = %v, want [4 5]", got)
	}
}

// A client resuming from another dev server (an earlier one, whose seq ran further) gets the
// whole ring.
func TestEventHub_AfterFromAnotherServer(t *testing.T) {
	h := newEventHub("b00t", 10)
	emitN(h, 2)
	backlog, _, cancel := h.subscribe(900)
	defer cancel()
	if got := backlogSeqs(t, backlog); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("backlog seqs = %v, want [1 2]", got)
	}
}

func TestEventHub_SlowSubscriberDropped(t *testing.T) {
	h := newEventHub("b00t", 10)
	_, ch, cancel := h.subscribe(0)
	defer cancel() // after the drop: a no-op
	done := make(chan struct{})
	go func() {
		defer close(done)
		emitN(h, 300)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("emit blocked on a subscriber that does not read")
	}
	n := 0
	for range ch { // ends only when the hub closed ch
		n++
	}
	if n != 256 {
		t.Errorf("the dropped subscriber's channel held %d events, want its buffer, 256", n)
	}
}

// eventStream reads the event stream at url line by line, pings included. It is closed when
// the test ends.
func eventStream(t *testing.T, url string) <-chan string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" ||
		res.Header.Get("Cache-Control") != "no-cache" {
		cancel()
		res.Body.Close()
		t.Fatalf("GET %s = %d %q %q", url, res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Cache-Control"))
	}
	lines := make(chan string, 100)
	read := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		res.Body.Close()
		<-read
	})
	go func() {
		defer close(read)
		defer close(lines)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			if sc.Text() == "" {
				continue
			}
			select {
			case lines <- sc.Text():
			case <-ctx.Done():
				return
			}
		}
	}()
	return lines
}

// nextEvent is the next data line of lines, decoded, skipping pings.
func nextEvent(t *testing.T, lines <-chan string) map[string]any {
	t.Helper()
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("the stream ended")
			}
			if l == ": ping" {
				continue
			}
			data, ok := strings.CutPrefix(l, "data: ")
			if !ok {
				t.Fatalf("line %q, want data: <json>", l)
			}
			var e map[string]any
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				t.Fatalf("data %q: %v", data, err)
			}
			return e
		case <-time.After(10 * time.Second):
			t.Fatal("no event in 10 s")
		}
	}
}

func TestServeEvents_HelloReplayPing(t *testing.T) {
	old := eventPing
	eventPing = 10 * time.Millisecond
	t.Cleanup(func() { eventPing = old })
	dir := t.TempDir()
	d := newDevServer(dir, "blog", "localhost:8080", io.Discard, io.Discard)
	first := d.hub.emit("run", map[string]any{"msg": "one"})
	d.hub.emit("run", map[string]any{"msg": "two"})
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close) // last: it waits for the streams, which end below
	t.Cleanup(func() { close(d.done) })

	lines := eventStream(t, srv.URL+"/_dev/events")
	hello := nextEvent(t, lines)
	if hello["type"] != "hello" || hello["dir"] != dir || hello["build"] != "building" || hello["app"] != "blog" ||
		hello["addr"] != "localhost:8080" || hello["boot"] != d.nonce || hello["child"] != nil || hello["seq"] != float64(2) ||
		hello["first"] != float64(1) || hello["gen"] != "" {
		t.Errorf("hello = %v", hello)
	}
	if v, ok := hello["alder"].(string); !ok || v == "" {
		t.Errorf("hello alder = %v", hello["alder"])
	}
	if _, err := time.Parse(time.RFC3339Nano, hello["since"].(string)); err != nil {
		t.Errorf("hello since = %v: %v", hello["since"], err)
	}
	for _, msg := range []string{"one", "two"} {
		if e := nextEvent(t, lines); e["type"] != "run" || e["msg"] != msg {
			t.Errorf("event = %v, want the run event %q", e, msg)
		}
	}
	for l := range lines { // nothing else is emitted: the next line is a ping
		if l != ": ping" {
			t.Errorf("line %q, want : ping", l)
		}
		break
	}

	resumed := eventStream(t, srv.URL+"/_dev/events?after="+strconv.FormatInt(first, 10))
	if e := nextEvent(t, resumed); e["type"] != "hello" {
		t.Errorf("first event = %v, want hello", e)
	}
	if e := nextEvent(t, resumed); e["type"] != "run" || e["msg"] != "two" {
		t.Errorf("event = %v, want the run event two", e)
	}

	// A live event reaches both streams.
	d.hub.emit("run", map[string]any{"msg": "three"})
	for _, s := range []<-chan string{lines, resumed} {
		if e := nextEvent(t, s); e["msg"] != "three" || e["seq"] != float64(3) {
			t.Errorf("event = %v, want the run event three", e)
		}
	}

	res, err := http.Post(srv.URL+"/_dev/events", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /_dev/events = %d, want 405", res.StatusCode)
	}
}
