package deploy

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"alder/internal/style"
)

func alwaysStyle() style.Style {
	return style.ForEnv(io.Discard, func(k string) string {
		if k == "ALDER_COLOR" {
			return "always"
		}
		return ""
	})
}

func TestStyled_PlainIsIdentity(t *testing.T) {
	var buf bytes.Buffer
	w, done := Styled(&buf, style.Plain)
	if w != io.Writer(&buf) {
		t.Error("Plain should return the writer itself")
	}
	done()
}

// The gate's rows are the plain rows with the status word marked; every other line is as it is.
func TestDeployGate_MarksStyled(t *testing.T) {
	st := alwaysStyle()
	var plain bytes.Buffer
	line(&plain, "check", "ok", "")
	line(&plain, "test", "ok", "116 passed")
	line(&plain, "test", "FAIL", "1 passed, 1 failed")
	line(&plain, "test", "SKIPPED", "--unsafe-skip-tests")
	text := "gate\n" + plain.String() + "plan\n  the target's schema is not known\n  DROPS DATA: x\nwarning: no backup\nartifact build/x (linux)\ndeployed abc to prod\ncurrent  abc\nbuilds\n  * abc  deployed\n    def  old\ndatabase 10 KB\nsilos    none yet\nunrelated line\n"
	var out bytes.Buffer
	w, done := Styled(&out, st)
	io.WriteString(w, text)
	done()
	got := out.String()
	for _, want := range []string{
		"  test     " + st.Green("ok") + "  116 passed\n",
		"  test     " + st.BoldRed("FAIL") + "  1 passed, 1 failed\n",
		"  test     " + st.Yellow("SKIPPED") + "  --unsafe-skip-tests\n",
		st.Bold("gate") + "\n",
		st.Bold("plan") + "\n",
		"  " + st.BoldRed("DROPS DATA") + ": x\n",
		st.Yellow("warning:") + " no backup\n",
		st.Bold("artifact build/x (linux)") + "\n",
		st.Bold("deployed abc to prod") + "\n",
		st.Bold("current") + "  abc\n",
		st.Green("  * abc  deployed") + "\n",
		st.Bold("database") + " 10 KB\n",
		st.Bold("silos") + "    none yet\n",
		"unrelated line\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%q", want, got)
		}
	}
	if style.Strip(got) != text {
		t.Errorf("Strip(styled) differs from the plain text:\n%q", style.Strip(got))
	}
}
