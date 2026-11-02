package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/feed"
	"alder/tui/internal/runner"
)

// shelfRoutes is the output of `alder routes --json` for the shelf app, sorted by path as alder
// sorts it.
const shelfRoutes = `[` +
	`{"kind":"page","path":"/","module":"Home","name":"page","file":"app/Home.al","line":1,"col":1},` +
	`{"kind":"msg","path":"/_m/Books/lend","module":"Books","name":"Lend","file":"app/Books.al","line":22,"col":3},` +
	`{"kind":"page","path":"/books/new","module":"Books","name":"page","file":"app/Books.al","line":40,"col":1},` +
	`{"kind":"page","path":"/books/{id}","module":"Books","name":"page","file":"app/Books.al","line":8,"col":1},` +
	`{"kind":"page","path":"/search","module":"Search","name":"page","file":"app/Search.al","line":2,"col":1}]`

// routesOptions is options() with a fake runner that answers `routes --json` with shelfRoutes.
func routesOptions() (Options, *runner.Fake) {
	f := &runner.Fake{}
	f.On("routes --json", shelfRoutes, 0)
	o := options(nil)
	o.Runner = f
	return o, f
}

func buildEvent(state string) feed.Event {
	return feed.Event{Type: "build", Build: &feed.Build{State: state}}
}

func TestRoutes_Screen(t *testing.T) {
	o, f := routesOptions()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5")
	if want := []string{"routes --json"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Fatalf("opening Routes called %q, want %q", f.Calls(), want)
	}
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if want := " 5 routes  4 hits"; lines[1] != want {
		t.Errorf("summary = %q, want %q", lines[1], want)
	}
	dim0 := "\x1b[2m0\x1b[0m"
	for _, want := range []string{
		"> page  /               Home.page    app/Home.al:1    1",
		"  msg   /_m/Books/lend  Books.Lend   app/Books.al:22  " + dim0,
		"  page  /books/new      Books.page   app/Books.al:40  " + dim0,
		"  page  /books/{id}     Books.page   app/Books.al:8   2",
		"  page  /search         Search.page  app/Search.al:2  1",
	} {
		if rowOf(frame, want) == "" {
			t.Errorf("no row %q in\n%s", want, frame)
		}
	}
	if !strings.HasSuffix(lines[len(lines)-1], "\x1b[2malder routes --json\x1b[0m") {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// A POST to a page's path, and a request no route matches, are no hit; a new request is counted
// as it arrives.
func TestRoutes_HitsFollowTheFeed(t *testing.T) {
	o, _ := routesOptions()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5")
	m.Update(eventMsg(access(14, clock, "5f3a2c24", "/books/7")))
	if r := rowOf(m.View(), "/books/{id}"); !strings.HasSuffix(r, " 3") {
		t.Errorf("after a request the row is %q, want 3 hits", r)
	}
	m.Update(eventMsg(access(15, clock, "5f3a2c25", "/nowhere")))
	if r := rowOf(m.View(), "/books/{id}"); !strings.HasSuffix(r, " 3") {
		t.Errorf("an unmatched request changed the row to %q", r)
	}
}

func TestRoutes_RefreshOnBuild(t *testing.T) {
	o, f := routesOptions()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5")
	if len(f.Calls()) != 1 {
		t.Fatalf("calls = %q", f.Calls())
	}
	run(m, eventMsg(buildEvent("building")))
	if len(f.Calls()) != 1 {
		t.Errorf("a build that has started refreshed the routes: %q", f.Calls())
	}
	run(m, eventMsg(buildEvent("ok")))
	if want := []string{"routes --json", "routes --json"}; !reflect.DeepEqual(f.Calls(), want) {
		t.Errorf("a build ok called %q, want %q", f.Calls(), want)
	}
	// The new routes replace the old ones.
	f.On("routes --json", `[{"kind":"page","path":"/about","module":"About","name":"page","file":"app/About.al","line":1,"col":1}]`, 0)
	run(m, eventMsg(buildEvent("ok")))
	if frame := m.View(); !strings.Contains(frame, "/about") || strings.Contains(frame, "/books/{id}") {
		t.Errorf("the routes were not replaced:\n%s", frame)
	}
}

// Start loads the routes before the tab is first opened, so the hits are there when it is.
func TestRoutes_LoadsOnStart(t *testing.T) {
	o, f := routesOptions()
	m := newRoot(o)
	run(m, m.Init()())
	if len(f.Calls()) != 1 {
		t.Fatalf("starting called %q, want one routes --json", f.Calls())
	}
	press(m, "5")
	if len(f.Calls()) != 1 {
		t.Errorf("opening the loaded tab called again: %q", f.Calls())
	}
}

func TestRoutes_Jump(t *testing.T) {
	o, _ := routesOptions()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5", "j", "j", "j")
	if c := rowOf(m.View(), " > "); !strings.Contains(c, "/books/{id}") {
		t.Fatalf("the cursor is on %q", c)
	}
	press(m, "enter")
	if m.tab != 0 {
		t.Fatalf("enter went to tab %d, want Requests", m.tab+1)
	}
	frame := m.View()
	for _, want := range []string{"/books/42", "/books/999"} {
		if rowOf(frame, want) == "" {
			t.Errorf("no row %q in\n%s", want, frame)
		}
	}
	for _, not := range []string{"GET   /  ", "/search", "/lend"} {
		if rowOf(frame, not) != "" {
			t.Errorf("the filtered table holds %q:\n%s", not, frame)
		}
	}
	if !strings.Contains(strings.Split(frame, "\n")[1], "route /books/{id}") {
		t.Errorf("the strip does not show the route filter: %q", strings.Split(frame, "\n")[1])
	}
	teatest.RequireEqualOutput(t, []byte(frame))
	// esc clears it.
	press(m, "esc")
	if frame := m.View(); rowOf(frame, "/search") == "" || strings.Contains(frame, "route /books") {
		t.Errorf("esc did not clear the route filter:\n%s", frame)
	}
}

// The text filter and the route filter both apply, and a route no request has hit says so.
func TestRoutes_FilterWithText(t *testing.T) {
	o, _ := routesOptions()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5", "j", "j", "j", "enter", "/", "9", "9", "enter")
	frame := m.View()
	if rowOf(frame, "/books/999") == "" || rowOf(frame, "/books/42") != "" {
		t.Errorf("text and route filters do not combine:\n%s", frame)
	}
	press(m, "esc") // clears both
	if rowOf(m.View(), "/search") == "" {
		t.Errorf("esc left a filter on:\n%s", m.View())
	}
	press(m, "5", "k", "enter") // the cursor was on /books/{id}; /books/new is above it
	if f := m.View(); !strings.Contains(f, "no requests match route /books/new") {
		t.Errorf("an empty filtered table says nothing about the route:\n%s", f)
	}
}

func TestRoutes_OpenInBrowser(t *testing.T) {
	var opened []string
	o, _ := routesOptions()
	o.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "5", "j", "j", "j", "j", "o") // /search, a page with no parameter
	press(m, "k", "o")                     // /books/{id}: not openable, so o opens the app
	press(m, "k", "k", "o")                // the message /_m/Books/lend: likewise
	want := []string{"http://localhost:8080/search", "http://localhost:8080/", "http://localhost:8080/"}
	if !reflect.DeepEqual(opened, want) {
		t.Errorf("opened %q, want %q", opened, want)
	}
}

func TestRoutes_Failure(t *testing.T) {
	f := &runner.Fake{}
	f.On("routes --json", "", 1)
	o := options(nil)
	o.Runner = f
	m := newRoot(o)
	press(m, "5")
	if frame := m.View(); !strings.Contains(frame, "alder routes --json failed (exit 1)") {
		t.Errorf("a failed run is not reported:\n%s", frame)
	}
	o.Runner = nil
	m = newRoot(o)
	press(m, "5") // no runner must not panic
	if frame := m.View(); !strings.Contains(frame, "no alder to run") {
		t.Errorf("no runner is not reported:\n%s", frame)
	}
}
