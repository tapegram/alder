package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/syntax"
	"alder/internal/types"
)

// The E11 drills (book 15, "An app from the command line"): testdata/drill/<task>/script.txt
// builds an E9 app from `alder new <app> --bare` with `alder new`, `alder edit` and `alder
// fill` only, no file edited by hand — T10 (teams) and T9 (bookings), each with its task's
// exact names, tables and fields (experiments/tasks/, the Names block). TestDrill replays both;
// each app has the task's names (drillNames), its own tests pass, the E9 hidden tests pass once
// added to the app as the E9 harness adds them (experiments/e9-feedback/verify.sh:
// app/Hidden.alder, `alder test -run '^Hidden '`), and its tests score at least 0.90 under
// `alder test --mutate`.

const drillScript = "../../testdata/drill/t10-teams/script.txt"

// drill is one recorded drill: the task, the app's name, and the names the task fixes.
type drill struct {
	task, app string
	names     drillNames
}

// drillNames are the names a task's Names block fixes: the tables and their columns
// (`name: Type`, the type as written in the table's module), exactly; the session's fields,
// exactly; the pages (module → path), exactly; each page's messages and forms, exactly, with each form's fields and each message's parameters.
type drillNames struct {
	tables  map[string][]string
	session []string
	pages   map[string]string
	msgs    map[string][]string
	forms   map[string][]string
	// formFields are each form's fields ("Teams.Index.NewTeam" → `name: Text.NonEmpty`), msgParams each
	// message's parameters ("Teams.Show.SendInvite" → `team: Team.TeamId`; none → no entry), exactly.
	formFields map[string][]string
	msgParams  map[string][]string
}

var drills = []drill{
	{task: "t10-teams", app: "teams", names: drillNames{
		tables: map[string][]string{
			"User":   {"id: UserId", "email: Text.Email", "name: Text.NonEmpty", "passwordHash: Text", "created: Time"},
			"Team":   {"id: TeamId", "name: Text.NonEmpty", "owner: User.UserId"},
			"Member": {"id: MemberId", "team: Team.TeamId", "user: User.UserId"},
			"Invite": {"id: InviteId", "team: Team.TeamId", "email: Text.Email", "status: Status"},
		},
		session: []string{"user: Maybe User.UserId"},
		pages:   map[string]string{"Teams.Index": "/teams", "Teams.Show": "/teams/{id}", "Invites.Index": "/invites", "Auth.Login": "/login"},
		msgs:    map[string][]string{"Teams.Index": {"Create"}, "Teams.Show": {"Remove", "SendInvite"}, "Invites.Index": {"Accept", "Decline"}, "Auth.Login": {"Submit"}},
		forms:   map[string][]string{"Teams.Index": {"NewTeam"}, "Teams.Show": {"NewInvite"}, "Auth.Login": {"Credentials"}},
		formFields: map[string][]string{"Teams.Index.NewTeam": {"name: Text.NonEmpty"}, "Teams.Show.NewInvite": {"email: Text.Email"},
			"Auth.Login.Credentials": {"email: Text.Email", "password: Text.Secret"}},
		msgParams: map[string][]string{"Invites.Index.Accept": {"id: Invite.InviteId"}, "Invites.Index.Decline": {"id: Invite.InviteId"}, "Teams.Show.Remove": {"user: User.UserId"}},
	}},
	{task: "t9-bookings", app: "bookings", names: drillNames{
		tables: map[string][]string{
			"User":    {"id: UserId", "email: Text.Email", "name: Text.NonEmpty", "passwordHash: Text", "created: Time"},
			"Room":    {"id: RoomId", "name: Text.NonEmpty"},
			"Booking": {"id: BookingId", "room: Room.RoomId", "owner: User.UserId", "from: Date", "to: Date"},
		},
		session:    []string{"user: Maybe User.UserId"},
		pages:      map[string]string{"Rooms.Index": "/rooms", "Rooms.Show": "/rooms/{id}", "Bookings.Index": "/bookings", "Bookings.Show": "/bookings/{id}", "Auth.Login": "/login"},
		msgs:       map[string][]string{"Rooms.Show": {"Book"}, "Bookings.Show": {"Cancel"}, "Auth.Login": {"Submit"}},
		forms:      map[string][]string{"Rooms.Show": {"NewBooking"}, "Auth.Login": {"Credentials"}},
		formFields: map[string][]string{"Rooms.Show.NewBooking": {"from: Date", "to: Date"}, "Auth.Login.Credentials": {"email: Text.Email", "password: Text.Secret"}},
		msgParams:  map[string][]string{},
	}},
}

// checkNames compares the app in dir with the names a task fixes.
func checkNames(t *testing.T, dir string, want drillNames) {
	t.Helper()
	p, err := generate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string][]string{}
	for _, tb := range p.Tables {
		for _, c := range tb.Columns {
			tables[tb.Name] = append(tables[tb.Name], c.Name+": "+c.TypeIn(tb.Module))
		}
	}
	if !reflect.DeepEqual(tables, want.tables) {
		t.Errorf("tables:\n%v\nwant:\n%v", tables, want.tables)
	}
	var session []string
	for _, f := range p.Session.Fields {
		session = append(session, f.Name+": "+types.String(f.Type))
	}
	if !slices.Equal(session, want.session) || p.Login != "Auth.Login" {
		t.Errorf("session %v, login %q; want %v, Auth.Login", session, p.Login, want.session)
	}
	pages, msgs, forms := map[string]string{}, map[string][]string{}, map[string][]string{}
	for _, pg := range p.Pages {
		pages[pg.Module] = pg.Path
		if len(pg.Msgs) > 0 {
			msgs[pg.Module] = pg.Msgs
		}
		if len(pg.Forms) > 0 {
			forms[pg.Module] = pg.Forms
		}
	}
	formFields, msgParams := map[string][]string{}, map[string][]string{}
	for _, pg := range p.Pages {
		m := p.Modules[pg.Module]
		for _, d := range m.Syntax.Decls {
			switch d := d.(type) {
			case *syntax.FormDecl:
				for _, f := range d.Fields {
					formFields[pg.Module+"."+d.Name] = append(formFields[pg.Module+"."+d.Name], f.Name+": "+drillSpan(m.Syntax.Lines, f.Type.GetSpan()))
				}
			case *syntax.MsgDecl:
				if d.Params != nil {
					for _, f := range d.Params.Fields {
						msgParams[pg.Module+"."+d.Name] = append(msgParams[pg.Module+"."+d.Name], f.Name+": "+drillSpan(m.Syntax.Lines, f.Type.GetSpan()))
					}
				}
			}
		}
	}
	for _, c := range []struct {
		what      string
		got, want any
	}{{"pages", pages, want.pages}, {"messages", msgs, want.msgs}, {"forms", forms, want.forms},
		{"form fields", formFields, want.formFields}, {"message parameters", msgParams, want.msgParams}} {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: %v, want %v", c.what, c.got, c.want)
		}
	}
}

// drillSpan is the source text of a one-line span.
func drillSpan(lines []string, sp diag.Span) string {
	if sp.Start.Line != sp.End.Line || sp.Start.Line > len(lines) {
		return "?"
	}
	l := lines[sp.Start.Line-1]
	return l[min(sp.Start.Col-1, len(l)):min(sp.End.Col-1, len(l))]
}

// drillStep is one command of a drill script: its arguments after `alder` and its standard
// input (a here-document: the lines after a `<<'EOF'` up to `EOF`).
type drillStep struct {
	line  int
	args  []string
	stdin string
}

// parseDrill reads a drill script: one command a line, `#` comments and blank lines skipped.
func parseDrill(src string) ([]drillStep, error) {
	lines := strings.Split(src, "\n")
	var steps []drillStep
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "#") {
			continue
		}
		words, err := drillWords(l)
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", i+1, err)
		}
		if len(words) < 2 || words[0] != "alder" {
			return nil, fmt.Errorf("line %d: a step is an `alder` command: %q", i+1, l)
		}
		st := drillStep{line: i + 1, args: words[1:]}
		if n := len(st.args); strings.HasPrefix(st.args[n-1], "<<") {
			delim := strings.Trim(strings.TrimPrefix(st.args[n-1], "<<"), "'\"")
			st.args = st.args[:n-1]
			var in strings.Builder
			for i++; ; i++ {
				if i >= len(lines) {
					return nil, fmt.Errorf("line %d: the here-document has no %s line", st.line, delim)
				}
				if lines[i] == delim {
					break
				}
				in.WriteString(lines[i] + "\n")
			}
			st.stdin = in.String()
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// drillWords splits a command line as a shell does: words separated by spaces, '…' and "…"
// taken as they are.
func drillWords(s string) ([]string, error) {
	var words []string
	var w strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ':
			if in {
				words = append(words, w.String())
				w.Reset()
				in = false
			}
		case '\'', '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return nil, fmt.Errorf("an unterminated %c", c)
			}
			w.WriteString(s[i+1 : i+1+j])
			i += j + 1
			in = true
		default:
			w.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, w.String())
	}
	return words, nil
}

// TestDrill replays each drill on a new bare app: every step succeeds; then the app checks, has
// its task's names, its own tests pass, its tests score at least 0.90, and the hidden tests
// pass.
func TestDrill(t *testing.T) {
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds apps and runs their mutants")
	}
	for _, d := range drills {
		t.Run(d.task, func(t *testing.T) {
			t.Parallel()
			script := "../../testdata/drill/" + d.task + "/script.txt"
			src, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			steps, err := parseDrill(string(src))
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), d.app)
			if code, _, errs := runNew(t, dir, "--bare"); code != 0 {
				t.Fatalf("alder new %s --bare: %s", d.app, errs)
			}
			before := appFiles(t, dir)
			for _, st := range steps {
				code, out, errs := alderIn(t, dir, st.stdin, st.args...)
				if code != 0 {
					t.Fatalf("%s:%d: alder %s: exit %d\n%s%s", script, st.line, strings.Join(st.args, " "), code, out, errs)
				}
				if st.args[0] == "test" && !strings.HasSuffix(out, " passed, 0 failed\n") {
					t.Fatalf("%s:%d: alder test:\n%s", script, st.line, out)
				}
			}
			if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || strings.Contains(out, "warning") {
				t.Fatalf("alder check after the drill: exit %d\n%s%s", code, out, errs)
			}
			t.Logf("the drill: %d commands; %d files written", len(steps), len(changedFiles(before, appFiles(t, dir))))
			checkNames(t, dir, d.names)

			// the score, on the app's own tests (before the hidden ones are added)
			var out, errb bytes.Buffer
			code := runTestTo([]string{dir, "--mutate"}, &out, &errb)
			m := scoreRe.FindStringSubmatch(out.String())
			if code != 0 || m == nil {
				t.Fatalf("alder test --mutate: exit %d\n%s%s", code, out.String(), errb.String())
			}
			t.Logf("the drill's app: %s", m[0])
			if m[4] < "0.90" {
				_, survivors, _ := strings.Cut(out.String(), "survived:\n")
				t.Errorf("score %s < 0.90; survivors:\n%s", m[4], survivors)
			}

			// the hidden tests, as the E9 harness runs them
			hidden, err := os.ReadFile("../../experiments/e9-feedback/hidden/" + d.task + "/Hidden.alder")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "app", "Hidden.alder"), hidden, 0o644); err != nil {
				t.Fatal(err)
			}
			total := strings.Count(string(hidden), "\ntest \"")
			out.Reset()
			errb.Reset()
			code = runTestTo([]string{dir, "-run", "^Hidden "}, &out, &errb)
			if want := fmt.Sprintf("%d passed, 0 failed\n", total); code != 0 || total == 0 || !strings.HasSuffix(out.String(), want) {
				t.Fatalf("the hidden tests: exit %d, want %q\n%s%s", code, want, out.String(), errb.String())
			}
			t.Logf("the hidden tests: %d of %d pass", total, total)
		})
	}
}

// TestDrill_Chapter: chapter 15's "An app from the command line" shows the drill's script, as
// it is: its `text` blocks, one after another, are the script's commands (the script without
// its first comment paragraph).
func TestDrill_Chapter(t *testing.T) {
	src, err := os.ReadFile(drillScript)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(src), "\n\n")
	if !ok {
		t.Fatal("the script has no header paragraph")
	}
	ch, err := os.ReadFile("../../book/15-generators.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(ch), "\n## An app from the command line\n")
	if !ok {
		t.Fatal("chapter 15 has no section \"An app from the command line\"")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var blocks []string
	for _, part := range strings.Split(section, "\n```text\n")[1:] {
		block, _, _ := strings.Cut(part, "\n```\n")
		if strings.HasPrefix(block, "alder ") || strings.HasPrefix(block, "# ") {
			blocks = append(blocks, block+"\n")
		}
	}
	if got := strings.Join(blocks, "\n"); got != body {
		t.Errorf("the chapter's blocks differ from %s:\n%s", drillScript, firstDifference(got, body))
	}
}

// firstDifference shows the first line where got and want differ.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < max(len(g), len(w)); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			return fmt.Sprintf("line %d:\n  chapter: %q\n  script:  %q", i+1, a, b)
		}
	}
	return "(equal)"
}
