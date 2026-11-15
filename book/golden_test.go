package book

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// goldenRE is a line `<!-- golden: <case>/<path> -->`: the fenced block after it is quoted from
// testdata/generate/want/<case>/files/<path>, or, for the path `output`, from what the case's
// command printed (its stdout, then its stderr).
var goldenRE = regexp.MustCompile(`^<!-- golden: ([A-Za-z0-9_-]+)/(\S+) -->$`)

// TestBook_Goldens: a block marked `<!-- golden: … -->` is lines of that golden, contiguous and
// unchanged (a leading `$ alder …` line of a transcript is left out), so the book shows the
// generator's own output for the resource pages, which no example app under examples/ can.
func TestBook_Goldens(t *testing.T) {
	chapters, err := filepath.Glob("*.md")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, ch := range chapters {
		b, err := os.ReadFile(ch)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i := 0; i < len(lines); i++ {
			m := goldenRE.FindStringSubmatch(lines[i])
			if m == nil {
				continue
			}
			if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "```") {
				t.Errorf("book/%s:%d: a golden marker is followed by a fenced block", ch, i+1)
				continue
			}
			var block []string
			j := i + 2
			for ; j < len(lines) && !strings.HasPrefix(lines[j], "```"); j++ {
				if len(block) == 0 && strings.HasPrefix(lines[j], "$ ") {
					continue
				}
				block = append(block, lines[j])
			}
			want := filepath.Join("..", "testdata", "generate", "want", m[1])
			var src string
			if m[2] == "output" {
				out, _ := os.ReadFile(filepath.Join(want, "stdout"))
				errb, _ := os.ReadFile(filepath.Join(want, "stderr"))
				src = string(out) + string(errb)
			} else {
				f, err := os.ReadFile(filepath.Join(want, "files", filepath.FromSlash(m[2])))
				if err != nil {
					t.Errorf("book/%s:%d: %v", ch, i+1, err)
					continue
				}
				src = string(f)
			}
			if !strings.Contains(src, strings.Join(block, "\n")+"\n") {
				t.Errorf("book/%s:%d: the block is not lines of golden %s/%s", ch, i+2, m[1], m[2])
			}
			n++
			i = j
		}
	}
	if n == 0 {
		t.Error("no chapter has a golden block")
	}
}
