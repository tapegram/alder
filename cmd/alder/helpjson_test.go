package main

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"alder/internal/generate"
)

// usageFlagRE finds flags in a usage string: `--json`, `-run`, `[--list T | --show T`, `|--ast`.
var usageFlagRE = regexp.MustCompile(`(?:^|[\s\[|(])(--?[a-z][a-z-]*)`)

func usageFlags(u string) []string {
	set := map[string]bool{}
	for _, m := range usageFlagRE.FindAllStringSubmatch(u, -1) {
		set[m[1]] = true
	}
	return sortedKeys(set)
}

// nodeFlags is a node's flags plus those of its subcommands that have no usage of their own.
func nodeFlags(c cmdSpec) []string {
	set := map[string]bool{}
	var add func(c cmdSpec)
	add = func(c cmdSpec) {
		for _, f := range c.Flags {
			set[f.Name] = true
		}
		for _, s := range c.Subcommands {
			if s.Usage == "" {
				add(s)
			}
		}
	}
	add(c)
	return sortedKeys(set)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestHelpJSON_MatchesUsage pins the structured tree to the text help: the same commands and
// generators, each node's usage string the one `--help` prints, and the same flags.
func TestHelpJSON_MatchesUsage(t *testing.T) {
	top := map[string]bool{}
	for _, c := range commandTree {
		top[c.Name] = true
		if want, ok := commandUsages[c.Name]; !ok {
			t.Errorf("command %q is in commandTree but not in commandUsages", c.Name)
		} else if c.Usage != want {
			t.Errorf("command %q: usage differs from commandUsages", c.Name)
		}
	}
	for name := range commandUsages {
		if !top[name] {
			t.Errorf("command %q has usage text but no commandTree entry", name)
		}
	}
	var gens map[string]bool
	for _, c := range commandTree {
		if c.Name != "new" {
			continue
		}
		gens = map[string]bool{}
		for _, s := range c.Subcommands {
			gens[s.Name] = true
			if s.Usage != newGeneratorUsages[s.Name] {
				t.Errorf("generator %q: usage differs from newGeneratorUsages", s.Name)
			}
		}
	}
	for name := range newGeneratorUsages {
		if !gens[name] {
			t.Errorf("generator %q has usage text but no commandTree entry", name)
		}
	}
	var walk func(path string, c cmdSpec)
	walk = func(path string, c cmdSpec) {
		if c.Summary == "" {
			t.Errorf("%s: no summary", path)
		}
		for _, f := range c.Flags {
			if f.Summary == "" {
				t.Errorf("%s %s: no summary", path, f.Name)
			}
		}
		if c.Usage != "" {
			got, want := strings.Join(nodeFlags(c), " "), strings.Join(usageFlags(c.Usage), " ")
			if got != want {
				t.Errorf("%s: flags in commandTree [%s] differ from its usage text [%s]", path, got, want)
			}
		}
		for _, s := range c.Subcommands {
			walk(path+" "+s.Name, s)
		}
	}
	for _, c := range commandTree {
		walk("alder "+c.Name, c)
	}
}

func TestHelpJSON_Output(t *testing.T) {
	var out bytes.Buffer
	if !wantsHelpJSON([]string{"help", "--json"}) {
		t.Fatal("main would not route `help --json` to writeHelpJSON")
	}
	if err := writeHelpJSON(&out); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version  string            `json:"version"`
		Commands []json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not one JSON object: %v\n%s", err, out.String())
	}
	if got.Version == "" || len(got.Commands) != len(commandTree) {
		t.Fatalf("version %q, %d commands; want a version and %d", got.Version, len(got.Commands), len(commandTree))
	}
	if strings.Contains(out.String(), `"flags":null`) || strings.Contains(out.String(), `"args":null`) {
		t.Fatalf("args and flags must be arrays, never null:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"name":"--list","value":"T"`) {
		t.Fatalf("`new page --list T` missing:\n%s", out.String())
	}
}

func TestHelp_SiloAndSilosHaveUsage(t *testing.T) {
	for _, args := range [][]string{{"help", "silos"}, {"new", "silo", "--help"}} {
		var out bytes.Buffer
		if !helpTo(args, &out) {
			t.Fatalf("%v: not handled", args)
		}
		if strings.Contains(out.String(), "usage: alder new <name> |") {
			t.Errorf("%v printed the general usage instead of its own:\n%s", args, out.String())
		}
	}
}

// TestHelpJSON_Golden pins `alder help --json` byte for byte: the generator nodes are rendered
// from generate.Specs (M6b) and must not change what alder.nvim and agents read.
func TestHelpJSON_Golden(t *testing.T) {
	var out bytes.Buffer
	writeHelpJSON(&out)
	want, err := os.ReadFile("../../testdata/help.json")
	if err != nil {
		t.Fatal(err)
	}
	got := out.Bytes()
	// the version varies by build; compare everything else
	re := regexp.MustCompile(`"version":"[^"]*"`)
	if !bytes.Equal(re.ReplaceAll(got, nil), re.ReplaceAll(want, nil)) {
		t.Fatalf("help --json changed:\n got %s\nwant %s", got, want)
	}
}

// TestHelpJSON_FromSpecs: every generator node in the tree is a Spec's, so a flag added to a
// Spec reaches help --json (and, through the usage pin, the usage text).
func TestHelpJSON_FromSpecs(t *testing.T) {
	for _, s := range generate.Specs() {
		group, sub, _ := strings.Cut(s.ID, " ")
		var node *cmdSpec
		for i := range commandTree {
			if commandTree[i].Name == group {
				for j := range commandTree[i].Subcommands {
					if commandTree[i].Subcommands[j].Name == sub {
						node = &commandTree[i].Subcommands[j]
					}
				}
			}
		}
		if node == nil {
			t.Fatalf("%s: not in commandTree", s.ID)
		}
		if node.Summary != s.Summary {
			t.Errorf("%s: summary %q, spec %q", s.ID, node.Summary, s.Summary)
		}
	}
}

// TestHelpJSON_InFlag: eval takes `--in Name` (repl's joins it with the REPL task).
func TestHelpJSON_InFlag(t *testing.T) {
	for _, c := range commandTree {
		if c.Name != "eval" {
			continue
		}
		for _, f := range c.Flags {
			if f.Name == "--in" {
				if f.Value != "Name" {
					t.Errorf("eval --in value %q, want Name", f.Value)
				}
				return
			}
		}
		t.Errorf("eval lists no --in flag")
	}
}
