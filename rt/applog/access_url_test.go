package applog

import (
	"bytes"
	"io"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

// A record carrying the raw URL is masked when its line is written, and not at all when it is
// filtered.
func TestAccess_URLMaskedLazily(t *testing.T) {
	u, _ := url.ParseRequestURI("/start?key=s3cret&seed=7")
	var b bytes.Buffer
	newAt(&b, Info, Text).Access(&Access{ID: "a", Method: "GET", URL: u, Status: 200})
	if got, want := b.String(), "2026/10/29 20:00:00 info: GET /start?key=••••&seed=7 200 0.0ms 0B rid=a\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	for _, f := range []Format{Text, JSON} {
		l := New(io.Discard, Warn, f)
		a := &Access{ID: "a", Method: "GET", URL: u, Status: 200}
		if n := testing.AllocsPerRun(1000, func() { l.Access(a) }); n != 0 {
			t.Errorf("%v: a filtered line with a URL to mask: %v allocations", f, n)
		}
	}
}

// An access line's path is at most MaxPath bytes, cut on a character boundary and ending "…".
func TestAccess_PathCapped(t *testing.T) {
	long := strings.Repeat("/abc", 1500) // 6000 bytes of short segments, nothing to mask
	u, _ := url.ParseRequestURI(long)
	var b bytes.Buffer
	newAt(&b, Info, Text).Access(&Access{ID: "a", Method: "GET", URL: u, Status: 200})
	line := b.String()
	if !strings.Contains(line, " GET "+long[:MaxPath]+"… 200 ") {
		t.Errorf("line of %d bytes: %.80q…", len(line), line)
	}
	// a cut inside a multi-byte character (the mask) backs up to its start
	p := strings.Repeat("a", MaxPath-1) + Mask
	got := (&Access{Path: p}).MaskedPath()
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "…") || len(got) > MaxPath+len("…") {
		t.Errorf("cut %q…", got[len(got)-10:])
	}
	if got := (&Access{Path: "/short"}).MaskedPath(); got != "/short" {
		t.Errorf("short path %q", got)
	}
}
