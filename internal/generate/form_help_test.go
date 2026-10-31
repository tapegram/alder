package generate

import (
	"strings"
	"testing"
)

// Issue 45: every field says what it means in one line, and every field but a switch has an
// example value that its own validation accepts; both are defined in specs.go, next to the
// Spec, and the editor's form shows them.
func TestSpecs_EveryFieldHasHelpAndExample(t *testing.T) {
	for _, s := range Specs() {
		for _, fs := range [][]Field{s.Args, s.Flags} {
			for _, f := range fs {
				h := f.HelpText()
				if h == "" || strings.Contains(h, "\n") {
					t.Errorf("%s %s: help %q is not one line", s.ID, f.Name, h)
				}
				if f.Kind == ArgBool {
					continue
				}
				if f.Example == "" {
					t.Errorf("%s %s: no example", s.ID, f.Name)
					continue
				}
				if ok, why := validate(f, f.Example, f.Choices, nil); !ok {
					t.Errorf("%s %s: example %q is invalid: %s", s.ID, f.Name, f.Example, why)
				}
			}
		}
	}
}

// Issue 45: the editor's form hides the agent-oriented flags; the CLI still takes them.
func TestSpecs_AgentFlags(t *testing.T) {
	want := []string{"new page --blank", "new form --msg", "new msg --in-for", "new msg --when", "new msg --id", "edit table --strict", "edit session --strict", "theme import --print"}
	var got []string
	for _, s := range Specs() {
		for _, f := range s.Args {
			if f.Agent {
				t.Errorf("%s: argument %s is marked for agents; only flags can be hidden", s.ID, f.Name)
			}
		}
		for _, f := range s.Flags {
			if f.Agent {
				got = append(got, s.ID+" --"+f.Name)
				if f.Required || f.Default != "" {
					t.Errorf("%s --%s: a hidden flag cannot be required or start set", s.ID, f.Name)
				}
			}
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("agent flags = %v", got)
	}
	sets, _, err := ParseArgs(SpecByID("new msg"), []string{"Home", "Hide", "--when", "True", "--id", "hide"})
	if err != nil || sets.One("when") != "True" || sets.One("id") != "hide" {
		t.Fatalf("the CLI must still take agent flags: %v %v", sets, err)
	}
}

// Issue 45: against an app that has pages, tables and declarations, every generator's form is
// ready as it opens (no cursor, nothing typed), so the editor's preview shows something at
// once: a required field starts at its first choice when it names something that exists, else
// at its example.
func TestForm_RequiredFieldsStartFilled(t *testing.T) {
	p, err := Load("../../examples/blog")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Specs() {
		fields, _, ready := Form(s, nil, nil, p)
		if !ready {
			t.Errorf("%s: not ready on open: %+v", s.ID, fields)
		}
		for _, f := range fields {
			if f.Required && (f.Value == "" || f.Source != "default") {
				t.Errorf("%s %s: required field starts %q from %q", s.ID, f.Name, f.Value, f.Source)
			}
			if f.Help == "" {
				t.Errorf("%s %s: no help in the form", s.ID, f.Name)
			}
		}
	}
	// an existing thing is the first choice; a new name is the example
	fields, _, _ := Form(SpecByID("remove table"), nil, nil, p)
	if fields[0].Value != fields[0].Choices[0] {
		t.Fatalf("remove table starts at %q, want the first table %q", fields[0].Value, fields[0].Choices[0])
	}
	spec := SpecByID("new capability")
	fields, _, _ = Form(spec, nil, nil, p)
	if fields[0].Value != spec.Args[0].Example || fields[0].Example != spec.Args[0].Example {
		t.Fatalf("new capability module starts at %q", fields[0].Value)
	}
	// the cursor and the user still win
	fields, _, _ = Form(SpecByID("new decl"), &Match{Sets: map[string]string{"module": "Layout"}}, map[string]string{"name": "x"}, p)
	if fields[0].Value != "Layout" || fields[0].Source != "here" || fields[1].Value != "x" || fields[1].Source != "you" {
		t.Fatalf("here and you: %+v", fields[:2])
	}
}

// Issue 45: a hidden flag is marked in the form, so the editor can leave it out.
func TestForm_AgentFlagMarked(t *testing.T) {
	fields, _, _ := Form(SpecByID("new msg"), nil, nil, nil)
	agent := map[string]bool{}
	for _, f := range fields {
		agent[f.Name] = f.Agent
	}
	if !agent["when"] || !agent["in-for"] || !agent["id"] || agent["page"] || agent["name"] {
		t.Fatalf("agent marks %v", agent)
	}
}
