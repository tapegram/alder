package main

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"alder/internal/generate"
)

// #35: help --json exposes which of a generator's args are required: `new route`'s module is
// optional, its path is not.
func TestHelpJSON_RequiredArgs(t *testing.T) {
	t.Parallel()
	for _, s := range generate.Specs() {
		group, sub, _ := strings.Cut(s.ID, " ")
		for _, c := range commandTree {
			for _, n := range c.Subcommands {
				if c.Name != group || n.Name != sub {
					continue
				}
				var want []bool
				for _, a := range s.Args {
					want = append(want, a.Required)
				}
				if !slices.Contains(want, true) {
					want = nil
				}
				if !slices.Equal(n.Required, want) {
					t.Errorf("%s: required %v, want %v", s.ID, n.Required, want)
				}
			}
		}
	}
}

// #35: the drill's asWritten drops the `Table.` of a table module's path, not the end of
// another name (`MyTable.X`).
func TestAsWrittenKeepsOtherTableNames(t *testing.T) {
	t.Parallel()
	if got := asWritten("Table.Team.TeamId -> MyTable.X"); got != "Team.TeamId -> MyTable.X" {
		t.Errorf("asWritten = %q", got)
	}
}

// #35: `alder new route --list T --show U` is a usage error (exit 2), not a silent --list.
func TestGenerate_NewRouteListAndShowConflict(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	dir := genApp(t, "scaffold")
	for _, args := range [][]string{
		{"new", "route", "/both", "--list", "Entry", "--show", "Entry"},
		{"new", "route", "About", "/about", "--list", "Entry", "--show", "Entry"},
		{"new", "route", "About", "/about", "--show", "Entry", "--from", "Entry"},
	} {
		code, out, errs := alderIn(t, dir, "", args...)
		if code != 2 || !strings.Contains(errs, "choose one of --list, --show and --from") || !strings.Contains(errs, "usage: alder new route") {
			t.Errorf("%v: exit %d\n%s%s", args, code, out, errs)
		}
	}
}

// #204: `alder new action` is the write route's button; `alder new msg` is an alias for one
// release: the same files, a notice on stderr, the same exit status and stdout, and no entry in
// help or `help --json`.
func TestGenerate_NewMsgIsAliasOfNewAction(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	var files [2]map[string]string
	var outs [2]string
	var errs [2]string
	for i, word := range []string{"action", "msg"} {
		dir := genApp(t, "scaffold")
		if code, _, e := alderIn(t, dir, "", "new", "route", "About", "/about"); code != 0 {
			t.Fatalf("new route: exit %d: %s", code, e)
		}
		code, out, e := alderIn(t, dir, "", "new", word, "About", "Archive")
		if code != 0 {
			t.Fatalf("new %s: exit %d: %s", word, code, e)
		}
		files[i], outs[i], errs[i] = appFiles(t, dir), out, e
	}
	if !reflect.DeepEqual(files[0], files[1]) {
		t.Errorf("new msg and new action wrote different files")
	}
	if outs[0] != outs[1] {
		t.Errorf("stdout differs:\n%q\n%q", outs[0], outs[1])
	}
	const notice = "alder new msg is now alder new action; new msg goes away in the next release\n"
	if strings.Contains(errs[0], "goes away") {
		t.Errorf("new action printed the notice: %q", errs[0])
	}
	if errs[1] != notice+errs[0] {
		t.Errorf("new msg stderr %q lacks %q", errs[1], notice)
	}
	if strings.Count(errs[1], "goes away") != 1 {
		t.Errorf("new msg stderr %q: want the notice once", errs[1])
	}
}

func TestHelpJSON_NewActionNotMsg(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	writeHelpJSON(&out)
	s := out.String()
	if !strings.Contains(s, `"name":"action"`) || strings.Contains(s, `"name":"msg"`) || strings.Contains(s, "new msg") {
		t.Errorf("help --json should list new action and not new msg")
	}
	for _, sp := range generate.Specs() {
		if sp.ID == "new msg" {
			t.Errorf("Specs lists the alias")
		}
	}
	if generate.SpecByID("new msg") != generate.SpecByID("new action") || generate.SpecByID("new action") == nil {
		t.Errorf("SpecByID(new msg) does not resolve to new action")
	}
}
