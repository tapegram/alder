package gen

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"alder/internal/ir"
	"alder/internal/types"
)

// Jobs (#168). A job value is an rt.Job{Kind, Params}: its qualified name and its parameters as
// JSON text. Each job's parameters record gets a codec over rt.JsonValue,
//
//	func jobenc__M__J__Params(v M__J__Params) rt.JsonValue
//	func jobdec__M__J__Params(v rt.JsonValue, path string) (M__J__Params, error)
//
// built from one such pair per record and custom type the parameters hold (named after the Go
// type, made an identifier, numbered when two make the same), and from rt/std's Job* pieces for
// the rest: a record is an object of its fields (by Alder name, in order), a List an array,
// Maybe null or the value, a custom type (Result too) {"tag": "C", "args": [payload]} (no
// argument for a constructor without one), a brand its base, a Time its milliseconds since
// the epoch, a Date "YYYY-MM-DD". Decoding is strict: a field of the wrong type, or missing
// (a Maybe too: Nothing is written as null), is an error naming its path ("user", "items[2].due"), and
// the worker marks the row dead with it.
//
// Jobs() lists every job as a jobs.Kind, whose Run decodes the parameters and calls the job's
// `run` arm; for a `silo k` job it first narrows the Env to the parameter k (db.Narrow), the
// silo jobs.RunOne binds the Task to. They are written to app__alder.go.

// jobCodecs are the codec functions the program's jobs need, in order of first need.
type jobCodecs struct {
	names map[string]string // Go type (or payload struct) → function suffix
	taken map[string]bool
	out   strings.Builder
}

// jobsSource is the codecs and Jobs() of p ("" for the codecs, and an empty Jobs, when p has
// no jobs).
func (g *gen) jobsSource(p *ir.Program) string {
	c := &jobCodecs{names: map[string]string{}, taken: map[string]bool{}}
	var kinds []string
	for _, j := range p.Jobs {
		pt := GoName(j.Params.Module, j.Params.Name)
		rec := &types.Record{Module: j.Params.Module, Name: j.Params.Name}
		for _, f := range j.Params.Fields {
			rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: f.Type})
		}
		dec := g.recordDec(c, pt, rec.Fields, j.Params.Module, j.Params.Name)
		g.recordEnc(c, pt, rec.Fields, j.Params.Module, j.Params.Name)
		narrow := "" // `silo k`: the job's Env narrowed to its parameter k (Keys), which RunOne binds
		if k := j.SiloParam; k != "" {
			v := "p." + Field(k)
			if g.boxed.has(j.Params.Module, j.Params.Name, k) {
				v = "*" + v
			}
			narrow = "db.Narrow(e, " + strconv.Quote(k) + ", " + v + ")\n"
		}
		k := fmt.Sprintf("{\nName: %q,\nRun: func(e *eff.Env, params string) (eff.Task[struct{}], error) {\n"+
			"v, err := std.JobParse(params)\nif err != nil {\nreturn eff.Task[struct{}]{}, err\n}\n"+
			"p, err := %s(v, \"\")\nif err != nil {\nreturn eff.Task[struct{}]{}, err\n}\n%sreturn %s(p), nil\n},\n"+
			"Retries: %d,\nTimeout: %d, // %s\n",
			j.Module+"."+j.Name, dec, narrow, GoName(j.Module, j.Name+".run"), j.Retries, int64(j.Timeout), j.Timeout)
		if j.Every != 0 {
			k += fmt.Sprintf("Every: %d, // %s\n", int64(j.Every), j.Every)
		}
		if j.Cron != "" {
			k += "Cron: " + strconv.Quote(j.Cron) + ",\n"
		}
		if j.SiloParam != "" {
			k += "SiloParam: " + strconv.Quote(j.SiloParam) + ",\n"
		}
		if j.Priority != 0 {
			k += fmt.Sprintf("Priority: %d,\n", j.Priority)
		}
		if len(j.Params.Fields) > 0 { // the --jobs task's `run` names them
			names := make([]string, len(j.Params.Fields))
			for i, f := range j.Params.Fields {
				names[i] = strconv.Quote(f.Name)
			}
			k += "Params: []string{" + strings.Join(names, ", ") + "},\n"
		}
		kinds = append(kinds, k+"}")
	}
	var b strings.Builder
	b.WriteString(c.out.String())
	b.WriteString("// Jobs are the app's jobs (#168), in module order then declaration order: each kind's\n" +
		"// qualified name, its options, and Run, which decodes a row's parameters and gives its `run`.\n")
	if len(kinds) == 0 {
		b.WriteString("func Jobs() []jobs.Kind {\nreturn []jobs.Kind{}\n}\n\n")
	} else {
		b.WriteString("func Jobs() []jobs.Kind {\nreturn []jobs.Kind{" + strings.Join(kinds, ", ") + "}\n}\n\n")
	}
	return b.String()
}

// jobRef is a job value: an rt.Job with the job's qualified name and its parameters encoded.
func (f *fn) jobRef(x *ir.JobRef, s *scope) string {
	j := x.Job
	enc := "jobenc__" + jobFuncName(GoName(j.Params.Module, j.Params.Name))
	prio := ""
	if j.Priority != 0 {
		prio = ", Priority: " + strconv.Itoa(j.Priority)
	}
	return "rt.Job{Kind: " + strconv.Quote(j.Module+"."+j.Name) + ", Params: rt.JsonEncode(" + enc + "(" + f.code(x.Params, s) + "))" + prio + "}"
}

// jobFuncName makes the Go type typ an identifier: every run of other characters one '_'.
func jobFuncName(typ string) string {
	var b strings.Builder
	under := false
	for _, r := range typ {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			b.WriteRune(r)
			under = false
		} else if !under && b.Len() > 0 {
			b.WriteByte('_')
			under = true
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}

// name is the function suffix of key (a Go type), and whether it is new (to be written).
func (c *jobCodecs) name(key string) (string, bool) {
	if n, ok := c.names[key]; ok {
		return n, false
	}
	base := jobFuncName(key)
	n := base
	for i := 2; c.taken[n]; i++ {
		n = fmt.Sprintf("%s_%d", base, i)
	}
	c.names[key], c.taken[n] = n, true
	return n, true
}

// enc is a Go func(T) rt.JsonValue encoding values of t.
func (g *gen) enc(c *jobCodecs, t types.Type) string {
	e := g.encExpr(c, t, "v")
	if f, ok := strings.CutSuffix(e, "(v)"); ok && !strings.ContainsAny(f, "(), ") { // a function of v alone
		return f
	}
	return "func(v " + GoType(t) + ") rt.JsonValue {\nreturn " + e + "\n}"
}

// encExpr encodes the Go value v of type t.
func (g *gen) encExpr(c *jobCodecs, t types.Type, v string) string {
	switch t := unbrand(t).(type) {
	case *types.Con:
		switch t.Name {
		case "Int":
			return "std.JsonInt(" + v + ")"
		case "Float":
			return "std.JsonFloat(" + v + ")"
		case "Bool":
			return "std.JsonBool(" + v + ")"
		case "Text":
			return "std.JsonString(" + v + ")"
		case "Date":
			return "std.JobEncDate(" + v + ")"
		case "Time":
			return "std.JobEncTime(" + v + ")"
		}
	case *types.App:
		if t.Con == "List" {
			return "std.JobEncList(" + v + ", " + g.enc(c, t.Args[0]) + ")"
		}
	case *types.Union:
		if a := maybeArg(t); a != nil {
			return "std.JobEncMaybe(" + v + ", " + g.enc(c, a) + ")"
		}
		return "jobenc__" + g.unionCodec(c, t) + "(" + v + ")"
	case *types.Record:
		typ := GoType(t)
		return "jobenc__" + g.recordEnc(c, typ, t.Fields, t.Module, t.Name) + "(" + v + ")"
	}
	panic("gen: a job parameter of type " + types.String(t) + " (E0049 refuses it)")
}

// dec is a Go func(rt.JsonValue, string) (T, error) decoding values of t.
func (g *gen) dec(c *jobCodecs, t types.Type) string {
	switch t := unbrand(t).(type) {
	case *types.Con:
		switch t.Name {
		case "Int", "Float", "Bool", "Text", "Date", "Time":
			return "std.JobDec" + t.Name
		}
	case *types.App:
		if t.Con == "List" {
			return "std.JobDecList(" + g.dec(c, t.Args[0]) + ")"
		}
	case *types.Union:
		if a := maybeArg(t); a != nil {
			return "std.JobDecMaybe(" + g.dec(c, a) + ")"
		}
		return "jobdec__" + g.unionCodec(c, t)
	case *types.Record:
		return g.recordDec(c, GoType(t), t.Fields, t.Module, t.Name)
	}
	panic("gen: a job parameter of type " + types.String(t) + " (E0049 refuses it)")
}

// recordEnc writes (once) the encoder of the record typ (its Go type: a record's, or a
// constructor's payload struct) with fields; module and name are the nominal record's ("" for
// another), whose boxed fields it reads through their pointer. It returns the function suffix.
func (g *gen) recordEnc(c *jobCodecs, typ string, fields []types.Field, module, name string) string {
	n, _ := c.name(typ)
	if c.names["enc "+typ] != "" {
		return n
	}
	c.names["enc "+typ] = n
	var ms []string
	for _, f := range fields {
		v := "v." + Field(f.Name)
		if name != "" && g.boxed.has(module, name, f.Name) {
			v = "(*" + v + ")"
		}
		ms = append(ms, "{Key: "+strconv.Quote(f.Name)+", Value: "+g.encExpr(c, f.Type, v)+"}")
	}
	members := ""
	if len(ms) > 0 {
		members = ", Members: []rt.JsonMember{" + strings.Join(ms, ", ") + "}"
	}
	fmt.Fprintf(&c.out, "// jobenc__%s is a %s as a job's parameters' JSON.\nfunc jobenc__%s(v %s) rt.JsonValue {\nreturn rt.JsonValue{Kind: rt.JsonObject%s}\n}\n\n", n, typ, n, typ, members)
	return n
}

// recordDec writes (once) the decoder of the record typ (see recordEnc) and returns its name.
func (g *gen) recordDec(c *jobCodecs, typ string, fields []types.Field, module, name string) string {
	n, _ := c.name(typ)
	if c.names["dec "+typ] != "" {
		return "jobdec__" + n
	}
	c.names["dec "+typ] = n
	var b strings.Builder
	fmt.Fprintf(&b, "// jobdec__%s reads a %s from a job's parameters' JSON at path.\nfunc jobdec__%s(v rt.JsonValue, path string) (%s, error) {\nvar r %s\n", n, typ, n, typ, typ)
	if len(fields) > 0 {
		b.WriteString("var err error\n")
	} else {
		b.WriteString("if v.Kind != rt.JsonObject {\nreturn r, std.JobWant(path, \"an object\", v)\n}\n")
	}
	for i, f := range fields {
		target := "r." + Field(f.Name)
		boxed := name != "" && g.boxed.has(module, name, f.Name)
		if boxed {
			target = fmt.Sprintf("f%d", i)
			fmt.Fprintf(&b, "var %s %s\n", target, GoType(f.Type))
		}
		fmt.Fprintf(&b, "if %s, err = std.JobField(v, path, %q, %s); err != nil {\nreturn r, err\n}\n", target, f.Name, g.dec(c, f.Type))
		if boxed {
			fmt.Fprintf(&b, "r.%s = &%s\n", Field(f.Name), target)
		}
	}
	b.WriteString("return r, nil\n}\n\n")
	c.out.WriteString(b.String())
	return "jobdec__" + n
}

// unionCodec writes (once) the encoder and decoder of the custom type u (Result included) and
// returns their suffix.
func (g *gen) unionCodec(c *jobCodecs, u *types.Union) string {
	typ := GoType(u)
	n, fresh := c.name(typ)
	if !fresh {
		return n
	}
	type arm struct {
		name  string
		arg   types.Type // nil: no argument
		get   string     // the argument's Go value, from v
		tag   string     // the Go case of the encoder's switch ("" for Result)
		build string     // the value, given the decoded argument a
		// encFn and decFn code a record payload ("" for another argument)
		encFn, decFn string
	}
	var arms []arm
	switch {
	case u.Module == "Prelude" && u.Name == "Result":
		e, a := GoType(u.Args[0]), GoType(u.Args[1])
		arms = []arm{
			{name: "Ok", arg: u.Args[1], get: "v.Val", build: "rt.Ok[" + e + ", " + a + "](a)"},
			{name: "Err", arg: u.Args[0], get: "v.Err", build: "rt.Err[" + e + ", " + a + "](a)"},
		}
	case u.Module == "Job":
		for _, ct := range u.Constructors() {
			arms = append(arms, arm{name: ct.Name, tag: "std.Job" + ct.Name, build: "std.JobState{Tag: std.Job" + ct.Name + "}"})
		}
	default:
		ta := ""
		if len(u.Args) > 0 {
			ta = typeArgs(u.Args)
		}
		for _, ct := range u.Constructors() {
			a := arm{name: ct.Name, tag: tagConst(u.Module, u.Name, ct.Name)}
			arg, _ := types.CtorArg(u, ct.Name)
			field := "v." + ctorField(ct.Name)
			switch r, isRec := arg.(*types.Record); {
			case arg == nil && ta == "":
				a.build = ctorName(u.Module, u.Name, ct.Name)
			case arg == nil:
				a.build = typ + "{Tag: " + a.tag + "}"
			case isRec:
				a.arg, a.get = arg, "(*"+field+")"
				pt, rm, rn := GoType(r), r.Module, r.Name // a nominal record: its boxed fields
				if r.Anonymous || r.Name == "" {          // the payload struct, by its Go name
					pt, rm, rn = payloadName(u.Module, u.Name, ct.Name)+ta, "", ""
				}
				a.encFn = "jobenc__" + g.recordEnc(c, pt, r.Fields, rm, rn)
				a.decFn = g.recordDec(c, pt, r.Fields, rm, rn)
				a.build = ctorName(u.Module, u.Name, ct.Name) + ta + "(a)"
			default:
				a.arg, a.get = arg, field+".Value"
				a.build = ctorName(u.Module, u.Name, ct.Name) + ta + "(a)"
			}
			arms = append(arms, a)
		}
	}
	// encoder
	var e strings.Builder
	fmt.Fprintf(&e, "// jobenc__%s is a %s as a job's parameters' JSON: {\"tag\": …, \"args\": […]}.\nfunc jobenc__%s(v %s) rt.JsonValue {\n", n, typ, n, typ)
	argEnc := func(a arm) string {
		if a.arg == nil {
			return "std.JobEncTagged(" + strconv.Quote(a.name) + ")"
		}
		if a.encFn != "" {
			return "std.JobEncTagged(" + strconv.Quote(a.name) + ", " + a.encFn + "(" + a.get + "))"
		}
		return "std.JobEncTagged(" + strconv.Quote(a.name) + ", " + g.encExpr(c, a.arg, a.get) + ")"
	}
	if arms[0].tag == "" { // Result
		fmt.Fprintf(&e, "if v.IsOk {\nreturn %s\n}\nreturn %s\n}\n\n", argEnc(arms[0]), argEnc(arms[1]))
	} else {
		e.WriteString("switch v.Tag {\n")
		for _, a := range arms[:len(arms)-1] {
			fmt.Fprintf(&e, "case %s:\nreturn %s\n", a.tag, argEnc(a))
		}
		fmt.Fprintf(&e, "}\nreturn %s\n}\n\n", argEnc(arms[len(arms)-1]))
	}
	// decoder
	var d strings.Builder
	var counts []string
	for _, a := range arms {
		k := 0
		if a.arg != nil {
			k = 1
		}
		counts = append(counts, fmt.Sprintf("%q: %d", a.name, k))
	}
	fmt.Fprintf(&d, "// jobdec__%s reads a %s from a job's parameters' JSON at path.\nfunc jobdec__%s(v rt.JsonValue, path string) (%s, error) {\nvar zero %s\ntag, args, err := std.JobTagged(v, path, map[string]int{%s})\nif err != nil {\nreturn zero, err\n}\nswitch tag {\n",
		n, typ, n, typ, typ, strings.Join(counts, ", "))
	for _, a := range arms {
		fmt.Fprintf(&d, "case %q:\n", a.name)
		if a.arg == nil {
			fmt.Fprintf(&d, "return %s, nil\n", a.build)
			continue
		}
		decA := a.decFn
		if decA == "" {
			decA = g.dec(c, a.arg)
		}
		fmt.Fprintf(&d, "a, err := %s(args[0], path+\".%s\")\nif err != nil {\nreturn zero, err\n}\nreturn %s, nil\n", decA, a.name, a.build)
	}
	d.WriteString("}\nreturn zero, nil\n}\n\n")
	c.out.WriteString(e.String())
	c.out.WriteString(d.String())
	return n
}
