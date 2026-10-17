package migrate

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Migration is one migration file.
type Migration struct {
	Name             string // "0003_add_comment"
	FromHash, ToHash string // the first 12 hex digits of old's and new's Schema.Hash
	Steps            []Step
	SQL              []string // the statements, without the trailing `;`
}

// hashLen is how many hex digits of a schema hash a migration file records.
const hashLen = 12

// Plan is Diff + SQL + hashes: the migration from old to new named name. For the first
// migration old is the empty Schema{}.
func Plan(old, new Schema, name string) (Migration, error) {
	m := Migration{Name: name}
	from, err := old.Hash()
	if err != nil {
		return m, err
	}
	to, err := new.Hash()
	if err != nil {
		return m, err
	}
	m.FromHash, m.ToHash = from[:hashLen], to[:hashLen]
	if why := SiloMove(old, new); why != "" {
		return m, errors.New(why)
	}
	if m.Steps, err = Diff(old, new); err != nil {
		return m, err
	}
	for _, st := range m.Steps {
		stmts, err := SQL(st, old, new)
		if err != nil {
			return m, err
		}
		m.SQL = append(m.SQL, stmts...)
	}
	return m, nil
}

// Render is the migration file: the header comment (the name, the hashes, one `-- step:` line
// per step, followed by a `-- DROPS DATA:` line for a step that loses data), a blank line, then
// the statements one per line, each ending in `;`. There is no BEGIN/COMMIT: the runtime runs a
// file in one transaction.
//
// The header is the file's leading lines that start with `--`; the first other line starts the
// statements (the runtime, which cannot import this package, splits a file the same way).
func Render(m Migration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- alder migration %s\n-- from %s\n-- to %s\n", m.Name, m.FromHash, m.ToHash)
	for _, s := range m.Steps {
		b.WriteString("-- step: " + s.String() + "\n")
		if s.DropsData != "" {
			b.WriteString("-- DROPS DATA: " + s.DropsData + "\n")
		}
	}
	b.WriteString("\n")
	for _, st := range m.SQL {
		b.WriteString(st + ";\n")
	}
	return b.String()
}

// ParseHeader reads the header of a migration file written by Render: its name, the two
// hashes, and the DROPS DATA lines in order. A leading byte-order mark is ignored.
func ParseHeader(text string) (name, fromHash, toHash string, dropsData []string, err error) {
	var header []string
	text = strings.TrimPrefix(text, "\ufeff") // a byte-order mark an editor may add
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "--") {
			break
		}
		header = append(header, line)
	}
	if len(header) == 0 {
		return "", "", "", nil, errors.New("not an alder migration: the first line is not `-- alder migration <name>`")
	}
	var ok bool
	if name, ok = strings.CutPrefix(header[0], "-- alder migration "); !ok || name == "" {
		return "", "", "", nil, errors.New("not an alder migration: the first line is not `-- alder migration <name>`")
	}
	for _, line := range header[1:] {
		switch {
		case strings.HasPrefix(line, "-- from "):
			fromHash = strings.TrimPrefix(line, "-- from ")
		case strings.HasPrefix(line, "-- to "):
			toHash = strings.TrimPrefix(line, "-- to ")
		case strings.HasPrefix(line, "-- DROPS DATA: "):
			dropsData = append(dropsData, strings.TrimPrefix(line, "-- DROPS DATA: "))
		}
	}
	if fromHash == "" || toHash == "" {
		return "", "", "", nil, fmt.Errorf("migration %s: the header lacks its `-- from` or `-- to` line", name)
	}
	return name, fromHash, toHash, dropsData, nil
}

var (
	slugRe = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)
	fileRe = regexp.MustCompile(`^([0-9]{4})_.+\.sql$`)
)

// NextName is "0004_<slug>" from the files present: one more than the highest NNNN_*.sql in
// dir (0001 when there is none, or no dir). slug is lower-case letters, digits and single
// underscores.
func NextName(dir string, slug string) (string, error) {
	if !slugRe.MatchString(slug) {
		return "", fmt.Errorf("migration name %q: use lower-case letters, digits and single underscores (add_comment)", slug)
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	high := 0
	for _, e := range entries {
		if m := fileRe.FindStringSubmatch(e.Name()); m != nil && !e.IsDir() {
			n, _ := strconv.Atoi(m[1])
			high = max(high, n)
		}
	}
	if high >= 9999 {
		return "", errors.New("migrations: 9999 files already; no next number")
	}
	return fmt.Sprintf("%04d_%s", high+1, slug), nil
}
