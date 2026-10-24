package generate

import (
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// EditSession generates `alder edit session add-field <name>:<Type>=<default>`: the field added
// at the end of App's `session { … }`, on its one line or before its closing line, the modules
// its type names imported into App. A session field needs a default (a new visitor's session,
// and every cookie written before the field existed, take it), so a field without one is
// refused, as is a name the session has and an App without a `session`. Session record literals
// written elsewhere are completed with the default, whatever their layout; a session built
// otherwise lacks the new field and no longer checks: those are the diagnostics the edit prints
// (the caller writes despite them, as `alder edit table` does). An error in the session declaration itself (an unknown type, a
// default of another type) refuses the edit (Edit.Guard): nothing is written.
func EditSession(p *Program, op string, args []string) ([]Edit, error) {
	return EditSessionChoose(p, op, args, "")
}

// EditSessionChoose is EditSession with `alder edit session`'s --choose: a key field (`org:
// Org.OrgId via Membership`, M15) is written `org: Maybe Org.OrgId via Membership` (a session
// starts without one: no default), its membership table checked (one `@ref` to the user table,
// one to the key's: the checker's E0032), and with choose (a page module) App gains `choose org
// = <Page>`, the page a request without the key is sent to (book 6, "Partitions").
func EditSessionChoose(p *Program, op string, args []string, choose string) ([]Edit, error) {
	arg := strings.Join(args, " ")
	via := ""
	if head, m, ok := strings.Cut(arg, " via "); ok {
		arg, via = strings.TrimSpace(head), strings.TrimSpace(m)
	}
	if op != "add-field" || arg == "" || len(args) != 1 && via == "" {
		return nil, Refusef("`alder edit session` has one operation: `add-field <name>:<Type>=<default>` (or `add-field <name>:<T>.<T>Id via <Membership>` for a key)")
	}
	f, err := ParseField(arg, false)
	if err != nil {
		return nil, Refusef("%v", err)
	}
	if choose != "" && via == "" {
		return nil, Refusef("--choose names the page that picks a `via` key: `add-field %s:<T>.<T>Id via <Membership> --choose %s`", f.Name, choose)
	}
	if via != "" {
		if !IsUpperIdent(via) {
			return nil, Refusef("`via %s`: a membership is a table, named by a capitalised identifier such as `Membership`", via)
		}
		if f.Default != "" {
			return nil, Refusef("session field `%s` is a key with `via %s`: it takes no default (a session starts without one, and a Membership row is checked when it is set)", f.Name, via)
		}
		if !strings.HasPrefix(f.Type, "Maybe ") {
			f.Type = "Maybe " + f.Type
		}
	} else if f.Default == "" {
		return nil, Refusef("session field `%s` needs a default (`%s:%s=…`): a new visitor's session, and every session stored before the field existed, takes it", f.Name, f.Name, f.Type)
	}
	app := p.Modules["App"]
	if app == nil || p.Session == nil {
		return nil, Refusef("there is no module `App` with a `session`: `alder new <app>` writes one")
	}
	if hasField(p.Session, f.Name) {
		return nil, Refusef("the session already has a field `%s`", f.Name)
	}
	if via != "" {
		if p.Session.User == nil {
			return nil, Refusef("`%s` is `via %s`, a membership of the signed-in user, but the session has no `user`: `alder new auth` adds one", f.Name, via)
		}
		if p.TableNamed(via) == nil {
			return nil, Refusef("`via %s`: `%s` is not a table of this app (its tables: %s)", via, via, tableNames(p))
		}
		for k := range p.Via {
			return nil, Refusef("the session's `%s` is `via %s` already: one `via` field per session (for now)", k, p.Via[k])
		}
		if choose != "" && p.PageNamed(choose) == nil {
			return nil, Refusef("--choose %s: `%s` is not a page of this app", choose, choose)
		}
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
	if via != "" {
		field = f.Name + ": " + f.Type + " via " + via
	}
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
	if choose != "" { // after the session: `choose org = Orgs.Index`
		e.insertBefore(sd.Span.End.Line+1, "\n-- a page that `requires "+f.Name+"` sends a session without one here, to pick one\nchoose "+f.Name+" = "+choose)
	}
	imports := newImports("App", p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, p, app.Syntax, imports.list())
	edits := []Edit{{Path: app.File, Body: e.result()}}
	def := f.Default
	if via != "" {
		def = "Nothing"
	}
	edits, err = completeSessionLiterals(p, edits, f.Name+": "+def)
	if err != nil {
		return nil, err
	}
	for i := range edits { // an error in the session itself (the type, the default) refuses
		if edits[i].Path == app.File {
			edits[i].Guard = func(d syntax.Decl) bool { _, ok := d.(*syntax.SessionDecl); return ok }
		}
	}
	return edits, nil
}

// completeSessionLiterals adds field (`theme: "light"`, the new field with its default) to the
// session record literals edits leave without it — the generated Logout test's, a test's
// `update X model { user: me }` — whatever their layout. The probe of edits finds them (the
// checker's "record literal for `Session` is missing").
func completeSessionLiterals(p *Program, edits []Edit, field string) ([]Edit, error) {
	probe, err := Probe(p.Dir, edits)
	if err != nil {
		return nil, err
	}
	name, _, _ := strings.Cut(field, ":")
	want := "record literal for `Session` is missing `" + name + "`"
	missing := func(m string) bool { // or, for a field without a default (a key), the record's
		return strings.HasPrefix(m, want) || strings.Contains(m, "record `App.Session` is missing field(s): "+name) && !strings.Contains(m, name+", ") && strings.HasSuffix(m, "field(s): "+name)
	}
	bodies := map[string]*srcEdit{}
	var order []string
	var lits []diag.Diagnostic
	for _, d := range probe.Diags {
		if d.Severity == "error" && missing(d.Message) {
			lits = append(lits, d)
		}
	}
	// right to left, so that several literals on one line each find their `}`
	slices.SortFunc(lits, func(a, b diag.Diagnostic) int {
		if a.Span.End.Line != b.Span.End.Line {
			return b.Span.End.Line - a.Span.End.Line
		}
		return b.Span.End.Col - a.Span.End.Col
	})
	for _, d := range lits {
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
		if !closeRecord(se, d.Span, field) {
			return nil, Refusef("internal error: the session record at %s:%d:%d is not where the checker says: add `%s` to it yourself", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, field)
		}
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

// closeRecord adds field to the record literal at span (its `}` the span's last character), after
// its last field, on one line or over several; false when the span does not end in `}`.
func closeRecord(e *srcEdit, span diag.Span, field string) bool {
	end := span.End.Col - 2 // the literal's `}`
	if l := e.current(span.End.Line); end < 0 || end >= len(l) || l[end] != '}' {
		return false
	}
	for n := span.End.Line; n >= span.Start.Line; n-- {
		l := e.current(n)
		if n == span.End.Line {
			l = l[:end]
		}
		code := strings.TrimRight(codeOf(l), " \t")
		if code == "" {
			continue
		}
		sep := ", "
		if strings.HasSuffix(code, "{") || strings.HasSuffix(code, ",") {
			sep = " "
		}
		text := sep + field
		if n == span.End.Line {
			text += " "
		}
		e.insertAt(n, len(code)+1, text)
		return true
	}
	return false
}

// codeOf is line without its trailing `-- comment` (a `--` inside a string literal is not one).
func codeOf(line string) string {
	inStr := false
	for i := 0; i < len(line); i++ {
		switch {
		case inStr && line[i] == '\\':
			i++
		case line[i] == '"':
			inStr = !inStr
		case !inStr && strings.HasPrefix(line[i:], "--"):
			return line[:i]
		}
	}
	return line
}
