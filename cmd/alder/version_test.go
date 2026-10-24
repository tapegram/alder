package main

import (
	"bytes"
	"regexp"
	"testing"
)

// `alder version` prints "alder <version> (go <go version>)" (RF2: the version strings
// themselves are internal/version's test); an argument is a usage error.
func TestVersion(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	if code := runVersionTo(nil, &out, &errb); code != 0 || errb.Len() > 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if !regexp.MustCompile(`^alder (v\d+\.\d+\.\d+\S*|devel\+[0-9a-f]{1,12}(-dirty)?|\(devel\)) \(go \d+\.\d+\S*\)\n$`).MatchString(out.String()) {
		t.Errorf("alder version printed %q", out.String())
	}
	out.Reset()
	if code := runVersionTo([]string{"--json"}, &out, &errb); code != 2 || out.Len() > 0 {
		t.Errorf("alder version --json: exit %d, stdout %q", code, out.String())
	}
}
