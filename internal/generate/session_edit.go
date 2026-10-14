package generate

import (
	"strings"

	"alder/internal/syntax"
)

// EditSession generates `alder edit session add-field <name>:<Type>=<default>`: the field added
// at the end of App's `session { … }`, on its one line or before its closing line, the modules
// its type names imported into App. A session field needs a default (a new visitor's session,
// and every cookie written before the field existed, take it), so a field without one is
// refused, as is a name the session has and an App without a `session`. Session record literals
// written elsewhere (a test's `update X model { user: me }`) lack the new field and no longer
// check: they are the diagnostics the edit prints (the caller writes despite them, as `alder
// edit table` does).
func EditSession(p *Program, op string, args []string) ([]Edit, error) {
	if op != "add-field" || len(args) != 1 {
		return nil, Refusef("`alder edit session` has one operation: `add-field <name>:<Type>=<default>`")
	}
	f, err := ParseField(args[0], false)
	if err != nil {
		return nil, Refusef("%v", err)
	}
	if f.Default == "" {
		return nil, Refusef("session field `%s` needs a default (`%s:%s=…`): a new visitor's session, and every session stored before the field existed, takes it", f.Name, f.Name, f.Type)
	}
	app := p.Modules["App"]
	if app == nil || p.Session == nil {
		return nil, Refusef("there is no module `App` with a `session`: `alder new <app>` writes one")
	}
	if hasField(p.Session, f.Name) {
		return nil, Refusef("the session already has a field `%s`", f.Name)
	}
	var sd *syntax.SessionDecl
	for _, d := range app.Syntax.Decls {
		if s, ok := d.(*syntax.SessionDecl); ok {
			sd = s
		}
	}
	if sd == nil {
		return nil, Refusef("`App` has no `session { … }` declaration")
	}
	e := newSrcEdit(app.Source)
	field := f.Source()
	if sd.Span.Start.Line == sd.Span.End.Line {
		l := e.line(sd.Span.Start.Line)
		open, close := strings.Index(l, "{"), sd.Span.End.Col-2
		if open < 0 || close >= len(l) || close < open || l[close] != '}' {
			return nil, Refusef("`App`'s session is not `session { … }` where the parser says: add the field yourself")
		}
		inner := strings.TrimSuffix(strings.TrimSpace(l[open+1:close]), ",")
		if inner != "" {
			inner += ", "
		}
		e.replaceLine(sd.Span.Start.Line, l[:open]+"{ "+inner+field+" }"+l[close+1:])
	} else {
		end := sd.Span.End.Line
		if strings.TrimSpace(e.line(end)) != "}" {
			return nil, Refusef("`App`'s session does not end with `}` on a line of its own: add the field yourself")
		}
		ind := "  "
		if n := len(sd.Fields); n > 0 {
			last := sd.Fields[n-1].Span
			ind = indentOf(e.line(last.Start.Line))
			if t := strings.TrimRight(e.line(last.End.Line), " "); !strings.HasSuffix(t, ",") {
				e.replaceLine(last.End.Line, t+",")
			}
		}
		e.insertBefore(end, ind+field+",")
	}
	imports := newImports("App", p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, app.Syntax, imports.list())
	edits := []Edit{{Path: app.File, Body: e.result()}}
	return completeSessionLiterals(p, edits, f.Name+": "+f.Default)
}

// completeSessionLiterals adds field (`theme: "light"`, the new field with its default) to the
// session record literals edits leave without it — the generated Logout test's, a test's
// `update X model { user: me }` — where a literal is written on one line ending in `}`. The
// probe of edits finds them (the checker's "record literal for `Session` is missing"); a
// literal written otherwise is left, and reported by the check after the edit.
func completeSessionLiterals(p *Program, edits []Edit, field string) ([]Edit, error) {
	probe, err := Probe(p.Dir, edits)
	if err != nil {
		return nil, err
	}
	name, _, _ := strings.Cut(field, ":")
	want := "record literal for `Session` is missing `" + name + "`"
	bodies := map[string]*srcEdit{}
	var order []string
	for _, d := range probe.Diags {
		if d.Severity != "error" || !strings.HasPrefix(d.Message, want) || d.Span.Start.Line != d.Span.End.Line {
			continue
		}
		se := bodies[d.Span.File]
		if se == nil {
			src := ""
			for _, ed := range edits {
				if ed.Path == d.Span.File {
					src = ed.Body
				}
			}
			if src == "" {
				for _, m := range p.Modules {
					if m.File == d.Span.File {
						src = m.Source
					}
				}
			}
			if src == "" {
				continue
			}
			se = newSrcEdit(src)
			bodies[d.Span.File] = se
			order = append(order, d.Span.File)
		}
		l := se.line(d.Span.Start.Line)
		end := d.Span.End.Col - 2 // the literal's `}`
		if end < 0 || end >= len(l) || l[end] != '}' {
			continue
		}
		before := strings.TrimRight(l[:end], " ")
		sep := ", "
		if strings.HasSuffix(before, "{") {
			sep = " "
		}
		se.replaceLine(d.Span.Start.Line, before+sep+field+" "+l[end:])
	}
	for _, file := range order {
		body := bodies[file].result()
		found := false
		for i := range edits {
			if edits[i].Path == file {
				edits[i].Body, found = body, true
			}
		}
		if !found {
			edits = append(edits, Edit{Path: file, Body: body})
		}
	}
	return edits, nil
}
