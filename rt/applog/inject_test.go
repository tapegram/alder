package applog

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

// Forged is a message carrying user text that tries to pass for an access line of its own.
const Forged = "signup bob\n2026/10/30 10:00:00 info: GET /admin/export 200 1.0ms 0B rid=abc"

// noForgedLines fails unless the text output's every line but those starting a stamp is
// indented: nothing a message carries starts at column 0.
func noForgedLines(t *testing.T, out string, lines int) {
	t.Helper()
	stamped := 0
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if l != "" && l[0] >= '0' && l[0] <= '9' {
			stamped++
			continue
		}
		if !strings.HasPrefix(l, "\t") {
			t.Errorf("a line at column 0: %q", l)
		}
	}
	if stamped != lines {
		t.Errorf("%d lines start with a stamp, want %d:\n%s", stamped, lines, out)
	}
}

func TestText_NoForgedLines(t *testing.T) {
	var b bytes.Buffer
	l := newAt(&b, Debug, Text)
	l.Log("info", Forged)
	l.Log("warn", "a\rb\r\nc\n\nd\n")
	l.Log("trace", "x\n2026/10/30 info: y") // an unknown level keeps its name, still one entry
	noForgedLines(t, b.String(), 3)
	want := "2026/10/29 20:00:00 info: signup bob\n\t2026/10/30 10:00:00 info: GET /admin/export 200 1.0ms 0B rid=abc\n" +
		"2026/10/29 20:00:00 warn: a\n\tb\n\tc\n\t\n\td\n" +
		"2026/10/29 20:00:00 info: trace: x\n\t2026/10/30 info: y\n"
	if b.String() != want {
		t.Errorf("got\n%q\nwant\n%q", b.String(), want)
	}
}

func TestText_ControlsEscaped(t *testing.T) {
	var b bytes.Buffer
	newAt(&b, Info, Text).Log("info", "a\x1b[2Jb\x00c\x7fd\u009be\tf é")
	if got, want := b.String(), "2026/10/29 20:00:00 info: a\\x1b[2Jb\\x00c\\x7fd\\u009be\tf é\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCutText(t *testing.T) {
	if CutText("short") != "short" {
		t.Error("short text cut")
	}
	s := strings.Repeat("a", MaxText-1) + "é" + strings.Repeat("b", 100)
	got := CutText(s)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") || len(got) > MaxText+len("…") || !strings.HasPrefix(got, strings.Repeat("a", MaxText-1)) {
		t.Errorf("cut to %d bytes, ending %q", len(got), got[len(got)-8:])
	}
}
