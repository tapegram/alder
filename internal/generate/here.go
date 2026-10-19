package generate

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"alder/internal/syntax"
)

// HereCtx is where the cursor is.
type HereCtx struct {
	Module   string // the module at the cursor ("Posts.Index")
	IsPage   bool   // the module declares a page
	Decl     string // the enclosing declaration's name, "" between declarations
	DeclKind string // "value", "type", "table", "page", "msg", "form", "capability", "test", "" (none)
	Hole     string // the hole's name when the cursor is on ?name
}

// Match is a Spec's prefill from the cursor, and the reason for it.
type Match struct {
	Sets map[string]string `json:"sets"`
	Why  string            `json:"why"`
}

// FieldState is one field of a generator's form.
type FieldState struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Flag     bool     `json:"flag"`
	Required bool     `json:"required"`
	Value    string   `json:"value"`
	Source   string   `json:"source"` // "here", "you", "" (empty)
	Valid    bool     `json:"valid"`
	Why      string   `json:"why,omitempty"`
	Choices  []string `json:"choices,omitempty"`
}

func inPage(c HereCtx) *Match {
	if !c.IsPage {
		return nil
	}
	return &Match{Sets: map[string]string{"page": c.Module}, Why: "the page at the cursor"}
}

func onTable(c HereCtx) *Match {
	if c.DeclKind != "table" {
		return nil
	}
	return &Match{Sets: map[string]string{"table": c.Decl}, Why: "the table at the cursor"}
}

func onDecl(c HereCtx) *Match {
	if c.Decl == "" || c.DeclKind == "table" || c.DeclKind == "page" {
		return nil
	}
	return &Match{Sets: map[string]string{"address": c.Module + "." + c.Decl}, Why: "the declaration at the cursor"}
}

func inModule(c HereCtx) *Match {
	if c.Module == "" {
		return nil
	}
	return &Match{Sets: map[string]string{"module": c.Module}, Why: "the module at the cursor"}
}

var moduleName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(\.[A-Z][A-Za-z0-9]*)*$`)

// Form merges you over here per field, validates each, and returns the fields in Spec order
// (Args, then Flags), the Sets the values make, and whether every field is valid. p may be nil
// (the app does not load): choices are then empty and validation is syntactic.
func Form(s *Spec, here *Match, you map[string]string, p *Program) ([]FieldState, Sets, bool) {
	values := map[string]string{}
	source := map[string]string{}
	if here != nil {
		for k, v := range here.Sets {
			values[k], source[k] = v, "here"
		}
	}
	for k, v := range you {
		values[k], source[k] = v, "you"
	}
	ready := true
	var out []FieldState
	add := func(f Field, flag bool) {
		v := strings.TrimSpace(values[f.Name])
		st := FieldState{Name: f.Name, Kind: f.Kind.String(), Flag: flag, Required: f.Required, Value: values[f.Name], Source: source[f.Name], Choices: choices(f, p)}
		st.Valid, st.Why = validate(f, v, st.Choices, p)
		if !st.Valid {
			ready = false
		}
		out = append(out, st)
	}
	for _, f := range s.Args {
		add(f, false)
	}
	for _, f := range s.Flags {
		add(f, true)
	}
	return out, SetsOf(s, values), ready
}

// SetsOf turns form text into Sets: a Variadic field or a Greedy flag is split into words, a
// Bool flag is set when "true", anything else is one value with its spaces kept.
func SetsOf(s *Spec, values map[string]string) Sets {
	sets := Sets{}
	put := func(f Field) {
		v := strings.TrimSpace(values[f.Name])
		switch {
		case v == "":
		case f.Kind == ArgBool:
			if v == "true" {
				sets[f.Name] = []string{""}
			}
		case f.Variadic || f.Greedy:
			sets[f.Name] = strings.Fields(v)
		default:
			sets[f.Name] = []string{v}
		}
	}
	for _, f := range s.Args {
		put(f)
	}
	for _, f := range s.Flags {
		put(f)
	}
	return sets
}

func choices(f Field, p *Program) []string {
	if f.Kind == ArgChoice {
		out := slices.Clone(f.Choices)
		sort.Strings(out)
		return out
	}
	if p == nil {
		return nil
	}
	var out []string
	switch f.Kind {
	case ArgPage:
		for _, pg := range p.Pages {
			out = append(out, pg.Module)
		}
	case ArgTable:
		for _, t := range p.Tables {
			out = append(out, t.Name)
		}
	case ArgModule:
		for name := range p.Modules {
			out = append(out, name)
		}
	case ArgDecl:
		for name, m := range p.Modules {
			if m.Syntax == nil {
				continue
			}
			for _, d := range m.Syntax.Decls {
				if vd, ok := d.(*syntax.ValueDecl); ok {
					out = append(out, name+"."+vd.Name)
				}
			}
		}
	default:
		return nil
	}
	sort.Strings(out)
	return out
}

func validate(f Field, v string, choices []string, p *Program) (bool, string) {
	if v == "" {
		if f.Required {
			return false, "required"
		}
		return true, ""
	}
	switch f.Kind {
	case ArgChoice:
		if !slices.Contains(choices, v) {
			return false, "one of " + strings.Join(choices, ", ")
		}
	case ArgPage:
		if p != nil && !slices.Contains(choices, v) {
			return false, "no page `" + v + "`"
		}
	case ArgTable:
		if p != nil && !slices.Contains(choices, v) {
			return false, "no table `" + v + "`"
		}
	case ArgDecl:
		if p != nil && !slices.Contains(choices, v) {
			return false, "no declaration `" + v + "`"
		}
	case ArgModule:
		if !moduleName.MatchString(v) {
			return false, "a module name is capitalised words joined by dots, as Posts.Index"
		}
	case ArgRoute:
		if !strings.HasPrefix(v, "/") {
			return false, "a path starts with /"
		}
	case ArgFieldList:
		for _, w := range strings.Fields(v) {
			if _, err := ParseField(w, f.Table); err != nil {
				return false, err.Error()
			}
		}
	case ArgBool:
		if v != "true" && v != "false" {
			return false, "true or false"
		}
	}
	return true, ""
}
