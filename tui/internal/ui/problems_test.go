package ui

import (
	"strings"
	"testing"
)

// The tail of an exited app's stderr is coloured as Logs colours it.
func TestProblems_TailStyled(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "exited")...)
	press(m, "p")
	v := m.View()
	for _, want := range []string{
		"\x1b[2m2026/11/01 14:20:05 \x1b[0m\x1b[32minfo:\x1b[0m migrating dev.db",
		"\x1b[2m2026/11/01 14:20:05 \x1b[0m\x1b[31merror:\x1b[0m migration 0003_loans.sql: table loans already exists",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q in\n%s", want, v)
		}
	}
}

// View reads the cursor and never writes it; the next Update clamps it.
func TestProblems_ViewPure(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "failed")...)
	press(m, "p")
	m.probs.cursor = 99
	first := m.View()
	if second := m.View(); second != first || m.probs.cursor != 99 {
		t.Errorf("View changed the cursor to %d or its output", m.probs.cursor)
	}
	if !strings.Contains(first, "> ") {
		t.Errorf("an out-of-range cursor marks no diagnostic:\n%s", first)
	}
	press(m, "j")
	if want := len(diags(m.ctx)) - 1; m.probs.cursor != want {
		t.Errorf("cursor after an Update = %d, want %d", m.probs.cursor, want)
	}
}
