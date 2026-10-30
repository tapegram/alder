package std

import (
	"bytes"
	"strings"
	"testing"

	"alder/rt/applog"
	"alder/rt/eff"
)

// Log.info with user text carrying a line break and a forged access line: the app's logger
// writes it as one entry, its continuation indented; a text over 8 KB is cut.
func TestLog_NoForgedLines(t *testing.T) {
	var b bytes.Buffer
	lg := applog.New(&b, applog.Info, applog.Text)
	name := "bob\n2026/10/30 10:00:00 info: GET /admin/export 200 1.0ms 0B rid=abc"
	if _, err := eff.Run(&eff.Env{Log: lg.Log}, eff.KTask, LogInfo("signup "+name)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], " info: signup bob") || lines[1] != "\t2026/10/30 10:00:00 info: GET /admin/export 200 1.0ms 0B rid=abc" {
		t.Errorf("lines %q", lines)
	}
	b.Reset()
	if _, err := eff.Run(&eff.Env{Log: lg.Log}, eff.KTask, LogWarn(strings.Repeat("x", 20000))); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(b.String(), "x"); n != applog.MaxText || !strings.HasSuffix(b.String(), "x…\n") {
		t.Errorf("%d x kept, ending %q", n, b.String()[b.Len()-8:])
	}
}
