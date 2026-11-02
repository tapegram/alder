package ui

import (
	"alder/tui/internal/store"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/feed"
)

// scrollback is the lines of a Logs frame: the lines between the rule under the strip
// and the rule above the footer that are not empty.
func scrollback(frame string) []string {
	lines := strings.Split(frame, "\n")
	var out []string
	for _, l := range lines[3 : len(lines)-2] {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestLogs_Follow(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "2")
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if lines[1] != " level info" {
		t.Errorf("strip = %q, want %q", lines[1], " level info")
	}
	if !strings.HasPrefix(lines[2], " ─") {
		t.Errorf("no rule under the strip: %q", lines[2])
	}
	if strings.Contains(frame, "effect Books.find") {
		t.Errorf("the effect line is shown at info:\n%s", frame)
	}
	if strings.Contains(frame, "/static/app.css") {
		t.Errorf("the debug access line is shown at info:\n%s", frame)
	}
	for _, want := range []string{
		"\x1b[2malder run: dev server on 127.0.0.1:8080 → app on 127.0.0.1:52507\x1b[0m",
		"\x1b[2m2026/11/01 14:02:12 \x1b[0m\x1b[33mwarn:\x1b[0m book 42 has no cover image",
		"\x1b[2m2026/11/01 14:02:12 \x1b[0m\x1b[32minfo:\x1b[0m showing book 42",
		"\x1b[2m2026/11/01 14:02:14 \x1b[0m\x1b[32minfo:\x1b[0m POST /books/42/lend 303 6.1ms 0B rid=5f3a2c20",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	ls := scrollback(frame)
	if len(ls) != 9 {
		t.Errorf("%d lines at info, want 9:\n%s", len(ls), frame)
	}
	if last := ls[len(ls)-1]; !strings.HasPrefix(last, " > panic: runtime error") {
		t.Errorf("the cursor does not follow the newest line: %q", last)
	}
	if !strings.HasSuffix(wideFooter(m), "\x1b[2mALDER_LOG=info alder run\x1b[0m") {
		t.Errorf("footer = %q", wideFooter(m))
	}
	// Moved up, the cursor stays on its line as lines arrive; G follows the newest again.
	press(m, "k")
	run(m, eventMsg(feed.Event{Type: "run", Seq: 14, Run: &feed.Run{Text: "alder run: building shelf"}}))
	if cur := cursorRow(m.View()); !strings.Contains(cur, "/search") {
		t.Errorf("the cursor moved off its line when a line arrived: %q", cur)
	}
	press(m, "G")
	if cur := cursorRow(m.View()); !strings.Contains(cur, "alder run: building shelf") {
		t.Errorf("G does not follow the newest line: %q", cur)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestLogs_LevelCycle(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "2")
	for _, want := range []string{"warn", "error", "debug"} {
		press(m, "L")
		if strip := strings.Split(m.View(), "\n")[1]; strip != " level "+want {
			t.Fatalf("after L the strip is %q, want level %s", strip, want)
		}
	}
	frame := m.View()
	for _, want := range []string{
		"\x1b[2m2026/11/01 14:02:12 debug: effect Books.find\x1b[0m",
		"GET /books/42 200 12.8ms 6377B rid=5f3a2c1f steps=5",
		"GET /static/app.css",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q at debug:\n%s", want, frame)
		}
	}
	if len(scrollback(frame)) != 11 {
		t.Errorf("%d lines at debug, want 11:\n%s", len(scrollback(frame)), frame)
	}
	if !strings.Contains(wideFooter(m), "ALDER_LOG=debug alder run") {
		t.Errorf("footer = %q", wideFooter(m))
	}
	press(m, "L")
	if strip := strings.Split(m.View(), "\n")[1]; strip != " level info" {
		t.Errorf("L from debug gives %q, want level info", strip)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestLogs_Regex(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "2", "/", "l", "e", "n", "d")
	if m.tab != 1 {
		t.Fatalf("typing the regex switched to tab %d", m.tab+1)
	}
	press(m, "enter")
	frame := m.View()
	ls := scrollback(frame)
	if len(ls) != 1 || !strings.Contains(ls[0], "POST /books/42/lend") {
		t.Fatalf("/lend shows %q", ls)
	}
	if strip := strings.Split(frame, "\n")[1]; strip != " level info  /lend/" {
		t.Errorf("strip = %q", strip)
	}
	// An invalid regexp keeps the old one and says why in the strip.
	press(m, "/", "(", "enter")
	after := m.View()
	strip := strings.Split(after, "\n")[1]
	if !strings.Contains(strip, "/lend/") || !strings.Contains(strip, "error parsing regexp: missing closing ): `lend(`") {
		t.Errorf("strip after an invalid regexp = %q", strip)
	}
	if got := scrollback(after); strings.Join(got, "\n") != strings.Join(ls, "\n") {
		t.Errorf("an invalid regexp changed the view:\n%s", after)
	}
	// x clears the regexp and the rid; the rid comes from Requests' l key.
	press(m, "1", "k", "k", "k", "l")
	if m.tab != 1 || !strings.Contains(strings.Split(m.View(), "\n")[1], "rid 5f3a2c1f") {
		t.Fatalf("l did not set the rid filter on tab %d:\n%s", m.tab+1, m.View())
	}
	press(m, "x")
	if strip := strings.Split(m.View(), "\n")[1]; strip != " level info" {
		t.Errorf("strip after x = %q", strip)
	}
	if n := len(scrollback(m.View())); n != 9 {
		t.Errorf("%d lines after x, want 9", n)
	}
}

func TestLogs_OpenRequest(t *testing.T) {
	o, asked := devServer(t, books42())
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "2", "k", "k", "k", "k", "k")
	if cur := cursorRow(m.View()); !strings.Contains(cur, "book 42 has no cover image") {
		t.Fatalf("the cursor is on %q, want the warn line", cur)
	}
	press(m, "enter")
	if m.tab != 1 {
		t.Errorf("enter switched to tab %d", m.tab+1)
	}
	if strings.Join(*asked, "|") != "GET /_dev/requests/5f3a2c1f" {
		t.Errorf("the dev server was asked %q", *asked)
	}
	frame := m.View()
	if !strings.Contains(frame, "← GET /books/42  200  12.8ms  rid 5f3a2c1f  route /books/{id}") {
		t.Errorf("enter does not open the request:\n%s", frame)
	}
	if !strings.Contains(wideFooter(m), "curl localhost:8080/_dev/requests/5f3a2c1f") {
		t.Errorf("footer = %q", wideFooter(m))
	}
	press(m, "esc")
	if !strings.Contains(strings.Split(m.View(), "\n")[1], "level info") {
		t.Errorf("esc does not return to Logs:\n%s", m.View())
	}
	// A line without a rid opens nothing.
	press(m, "G", "enter")
	if strings.Contains(m.View(), "←") || len(*asked) != 1 {
		t.Errorf("enter on a line without a rid opened a request:\n%s", m.View())
	}
}

func TestLogs_TruncateRune(t *testing.T) {
	text := strings.Repeat("界", 100)
	for _, limit := range []int{1, 2, 3, 4, 5, 31, 100, 299, 300, 301} {
		got := cutRunes(text, limit)
		if !utf8.ValidString(got) || len(got) > limit {
			t.Errorf("cutRunes(limit %d) = %d bytes, valid %v", limit, len(got), utf8.ValidString(got))
		}
	}
	// A line far wider than the terminal renders as valid UTF-8 and fits the width.
	ln := store.Line{Text: strings.Repeat("界", 5000), Run: true}
	for _, w := range []int{7, 20, 79} {
		out := renderLine(ln, w)
		if !utf8.ValidString(out) {
			t.Errorf("width %d: invalid UTF-8", w)
		}
		if ansi.StringWidth(ansi.Truncate(out, w, "…")) > w {
			t.Errorf("width %d: cut line is wider", w)
		}
	}
}

// With a ring of 3 and 5 lines, the lines the ring drops leave no stale position, and the cursor
// stays on a line, whether it follows or sits on a line that is overwritten.
func TestLogs_RingWrap(t *testing.T) {
	m := newRoot(options(nil))
	m.ctx.Logs = store.NewLogs(3)
	press(m, "2")
	add := func(i int) {
		run(m, eventMsg(feed.Event{Type: "run", Seq: int64(i), Run: &feed.Run{Text: fmt.Sprintf("alder run: line %d", i)}}))
		m.View()
	}
	s := m.screens[1].(*logsScreen)
	check := func(when string) {
		t.Helper()
		lg := m.ctx.Logs
		if len(s.idx) > 3 {
			t.Errorf("%s: idx = %v, more than the ring keeps", when, s.idx)
		}
		for _, p := range s.idx {
			if p < lg.Start() || p >= lg.End() {
				t.Errorf("%s: idx %v holds %d outside [%d,%d)", when, s.idx, p, lg.Start(), lg.End())
			}
		}
		if c := s.cursor(); len(s.idx) > 0 && (c < 0 || c >= len(s.idx)) {
			t.Errorf("%s: cursor %d out of idx %v", when, c, s.idx)
		}
	}
	for i := 1; i <= 3; i++ {
		add(i)
	}
	press(m, "k", "k")
	m.View()
	for i := 4; i <= 5; i++ {
		add(i)
		check(fmt.Sprintf("after line %d", i))
	}
	frame := m.View()
	if strings.Contains(frame, "line 1") || strings.Contains(frame, "line 2") || !strings.Contains(frame, "line 3") {
		t.Errorf("the frame is not the ring's three lines:\n%s", frame)
	}
	if cur := cursorRow(frame); !strings.Contains(cur, "line 3") {
		t.Errorf("the cursor of an overwritten line should land on the oldest kept, got %q", cur)
	}
	press(m, "G")
	add(6)
	check("following")
	if cur := cursorRow(m.View()); !strings.Contains(cur, "line 6") {
		t.Errorf("following: cursor row %q", cur)
	}
}

func TestLogs_RidStripHiddenCount(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "1", "k", "k", "k", "l")
	strip := func() string { return strings.Split(m.View(), "\n")[1] }
	// The fixture already has four newer lines of other requests.
	if s := strip(); s != " level info  rid 5f3a2c1f · 4 newer lines hidden (x to clear)" {
		t.Fatalf("strip = %q", s)
	}
	m.Update(eventMsg(access(100, clock, "other001", "/a")))
	if s := strip(); !strings.Contains(s, "rid 5f3a2c1f · 5 newer lines hidden (x to clear)") {
		t.Errorf("strip with five hidden = %q", s)
	}
	m.Update(eventMsg(access(101, clock, "other001", "/b")))
	if s := strip(); !strings.Contains(s, "rid 5f3a2c1f · 6 newer lines hidden (x to clear)") {
		t.Errorf("strip with six hidden = %q", s)
	}
	press(m, "x")
	if s := strip(); strings.Contains(s, "rid") {
		t.Errorf("strip after x = %q", s)
	}
}
