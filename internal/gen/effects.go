package gen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Effect and database emission.
//
// A Query or Task is a Go value of type eff.Query[T] or eff.Task[T] built with the constructors of
// alder/rt/eff; nothing runs while it is built. The combinators (eff.Bind, Map, All, …) take and
// return Tasks, so an effect node is first rendered in its natural form (effNode: usually a
// Task), then:
//   - where a Query is expected (the node's own type is Query) a Task form is wrapped once,
//     eff.Q(…);
//   - where a Task is needed (an operand of Bind, Map or All, a Bind's continuation result) a
//     Query-typed expression that is not a node is converted, eff.T(…) (task).
//
//	EffPure            eff.Pure(v)
//	EffBind            eff.Bind(task(e), func(x A) eff.Task[B] { …; return task(body) })
//	EffMap             eff.Map(fn, task(e))
//	EffAll             eff.All([]eff.Task[A]{task(e1), …}); a list that is no literal:
//	                   eff.All(list) for Tasks, std.QueryAll(list) for Queries
//	EffFail/NotFound   eff.Fail[T](msg), eff.NotFound[T]()
//	DbGet, DbSelect    eff.Call("Db.get Todo", false, func(_e *eff.Env) (R, error) {
//	  (run), DbRun,      return db.GetRow(_e.Ctx, _e.Store.(db.Store), db.TxOf(_e), …) })
//	  DbInsert, …      (write is true for insert, update, delete); Db.insert passes the row's
//	                   column values (insertvals__M__T) and _e.Now(), which eff.Run never
//	                   leaves nil; Db.update and Db.delete produce struct{}{}
//	DbSelect (a value) db.Offset(n, db.Limit(n, db.OrderBy(o, db.Where(c, db.From(table)))))
//	                   built on Base when set
//	DbMine             db.Mine(table): a Select whose owner predicate db.All/First/CountRows add
//	                   from the running effect's session (the trace notes "Db.mine Todo")
//	DbGetMine, …       eff.Call("Db.getMine Todo", false, func(_e) { return db.GetMineRow(…, id) }),
//	                   Db.updateMine/deleteMine/insertMine likewise (db.UpdateMineRow,
//	                   DeleteMineRow, InsertMineRow with insertminevals__M__T(row)): write true;
//	                   the user is the session of _e (the descriptor's User), a row that is not
//	                   the user's eff.ErrNotFound
//	Cond               db.CEq("col", enc(v)), …; and/or of a literal list: db.CAnd([]db.Cond{…}),
//	                   of a list value: db.CAnd(list) (CAnd of none is true, COr of none false)
//	Ord                db.Asc("col") / db.Desc("col")
//	CapCall            eff.Call("Cap.entry", write, func(_e) { _c, _ok := _e.Caps["M.Cap"].(Cap__M__Cap);
//	                   if !_ok { panic("alder: capability \"M.Cap\" is not wired") };
//	                   _v, _err := _c.Entry(_e.Ctx, args…); return _v, eff.CapError("Cap.entry", _err) }),
//	                   write true for a Task entry, false for a Query entry (the registry keys
//	                   are the Capabilities variable); the argument of a one-parameter entry is
//	                   its Args__ record (caps.go); an adapter's error becomes *eff.ErrCapability
//	                   (a failure passes through), which Test.run observes as Err "Cap.entry: msg"
//	TableRef           table__M__T
//	TestRun            eff.Call("Test.run", true, func(_e *eff.Env) (R, error) { _v, _msg, _ok, _tr,
//	                   _err := rtesting.Inner(_e, task(t)); …; R{Value: rt.Ok/Err…, Trace: _tr} })
//	                   (R the result record { value: Result Text a, trace: List Text })
//	TestTrace          eff.Call("Test.trace", false, func(_e) { return rtesting.Trace(_e), nil })
//	TestFake           eff.Call("Test.fake Stripe", false, func(_e) { _e.Caps["Payments.Stripe"] =
//	                   Fakes["Payments.Stripe"](); return struct{}{}, nil }) in a test build (the
//	                   Caps of the running test, its own copy: rtesting)
//
// The Test nodes exist only in `test` bodies, which only a test build emits (Options.Tests):
// one reaching the emitter in a production build is an internal error (testNode).
//
// Values compared with a column are encoded with the column's encoder (encode): Bool, Date, Time
// and payload-less unions (name__M__U) have one, Maybe goes through db.EncodeMaybe, Text, Int and
// Float are stored as they are. Calls read the transaction of the running effect (db.TxOf, which
// begins a Task's transaction on its first store access): inside a Task every read sees the
// Task's writes. A capability gets _e.Ctx, which carries the Env (eff.EnvFrom, db.TxFrom).

// effect is an effect, database or condition node as a Go expression of its own type.
func (f *fn) effect(e ir.Expr, s *scope) string {
	switch x := e.(type) {
	case *ir.DbSelect:
		if x.Terminal == ir.SelNone {
			return f.selectExpr(x, s)
		}
	case *ir.Cond:
		return f.cond(x, s)
	case *ir.Ord:
		if x.Desc {
			return "db.Desc(" + strconv.Quote(x.Col.Field) + ")"
		}
		return "db.Asc(" + strconv.Quote(x.Col.Field) + ")"
	case *ir.ColumnRef: // Column t a is the column's name
		return strconv.Quote(x.Field)
	case *ir.TableRef:
		return tableName(x.Table.Module, x.Table.Name)
	case *ir.DbMine:
		return "db.Mine(" + tableName(x.Table.Module, x.Table.Name) + ")"
	}
	c, isTask := f.effNode(e, s)
	if isQuery(e.Type()) && isTask {
		return "eff.Q(" + c + ")"
	}
	return c
}

// isQuery reports whether t is `Query a`.
func isQuery(t types.Type) bool {
	app, ok := t.(*types.App)
	return ok && app.Con == "Query"
}

// effResult is the result type a of an effect type `Query a` or `Task a`.
func effResult(t types.Type) types.Type {
	if app, ok := t.(*types.App); ok && (app.Con == "Query" || app.Con == "Task") && len(app.Args) == 1 {
		return app.Args[0]
	}
	panic(fmt.Sprintf("gen: %s is not an effect type", types.String(t)))
}

// isEffNode reports whether e is an effect node effNode renders.
func isEffNode(e ir.Expr) bool {
	switch x := e.(type) {
	case *ir.EffPure, *ir.EffBind, *ir.EffMap, *ir.EffAll, *ir.EffFail, *ir.EffNotFound, *ir.CapCall,
		*ir.DbGet, *ir.DbRun, *ir.DbInsert, *ir.DbUpdate, *ir.DbDelete, *ir.TestRun, *ir.TestTrace, *ir.TestFake,
		*ir.DbGetMine, *ir.DbUpdateMine, *ir.DbDeleteMine, *ir.DbInsertMine, *ir.TestLogin, *ir.TestGet, *ir.TestSubmit, *ir.TestSubmitRaw, *ir.TestClick:
		return true
	case *ir.DbSelect:
		return x.Terminal != ir.SelNone
	}
	return false
}

// task is effect expression e (of type Query a or Task a) as a Go eff.Task[A].
func (f *fn) task(e ir.Expr, s *scope) string {
	if h, ok := f.hoisted(e); ok { // a mutate build (mutation.go)
		if isQuery(e.Type()) {
			return "eff.T(" + h + ")"
		}
		return h
	}
	if ms := f.mutantsOf(e); ms != nil { // a mutate build (mutation.go)
		return f.switchedExpr(e, ms, "eff.Task["+GoType(effResult(e.Type()))+"]", s, f.task)
	}
	if isEffNode(e) {
		c, isTask := f.effNode(e, s)
		if !isTask {
			return "eff.T(" + c + ")"
		}
		return c
	}
	c := f.code(e, s)
	if isQuery(e.Type()) {
		return "eff.T(" + c + ")"
	}
	return c
}

// effNode renders effect node e in its natural form, and whether that is an eff.Task (else it
// is an eff.Query).
func (f *fn) effNode(e ir.Expr, s *scope) (string, bool) {
	switch x := e.(type) {
	case *ir.EffPure:
		v := f.typedExpr(x.Value, s)
		if vr, ok := x.Value.(*ir.Var); ok && vr.Kind == ir.VarStd {
			if _, isFun := vr.Ty.(*types.Fun); isFun { // a generic Go function: say its type
				return "eff.Pure[" + GoType(x.Value.Type()) + "](" + v + ")", true
			}
		}
		return "eff.Pure(" + v + ")", true
	case *ir.EffBind:
		return f.bind(x, s), true
	case *ir.EffMap:
		fn := f.code(x.Fn, s)
		return "eff.Map(" + fn + ", " + f.task(x.Effect, s) + ")", true
	case *ir.EffAll:
		elem := effResult(x.Ty).(*types.App).Args[0] // x is Query (List a) or Task (List a)
		if l, ok := x.Effects.(*ir.List); ok {
			var es []string
			for _, el := range l.Elems {
				es = append(es, f.task(el, s))
			}
			return "eff.All([]eff.Task[" + GoType(elem) + "]{" + strings.Join(es, ", ") + "})", true
		}
		if x.Kind == ir.EffQuery {
			return "std.QueryAll(" + f.code(x.Effects, s) + ")", false
		}
		return "eff.All(" + f.code(x.Effects, s) + ")", true
	case *ir.EffFail:
		return "eff.Fail[" + GoType(effResult(x.Ty)) + "](" + f.code(x.Message, s) + ")", true
	case *ir.EffNotFound:
		return "eff.NotFound[" + GoType(effResult(x.Ty)) + "]()", true
	case *ir.DbGet:
		tb := x.Table
		return f.dbCall("Db.get "+tb.Name, false, x.Ty, s, func(env string, c *scope) string {
			return "db.GetRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " + f.code(x.ID, c) + ")"
		}), true
	case *ir.DbSelect:
		return f.run(x.Table.Name, x.Terminal, x.Ty, s, func(c *scope) string { return f.selectExpr(x, c) }), true
	case *ir.DbRun:
		return f.run(selectTable(x.Select.Type()), x.Terminal, x.Ty, s, func(c *scope) string { return f.code(x.Select, c) }), true
	case *ir.DbInsert:
		tb := x.Table
		return f.dbCall("Db.insert "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			row := ""
			if r, ok := x.Row.(*ir.Record); ok { // a literal: name its type by the alias
				row = f.record(r, insertName(tb.Module, tb.Name), c)
			} else {
				row = f.code(x.Row, c)
			}
			return "db.InsertRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " +
				insertVals(tb.Module, tb.Name) + "(" + row + "), " + env + ".Now())"
		}), true
	case *ir.DbUpdate:
		tb := x.Table
		return f.dbCall("Db.update "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			return "struct{}{}, db.UpdateRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " +
				f.code(x.ID, c) + ", " + f.code(x.Fn, c) + ", " + env + ".Now())"
		}), true
	case *ir.DbDelete:
		tb := x.Table
		return f.dbCall("Db.delete "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			return "struct{}{}, db.DeleteRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " + f.code(x.ID, c) + ")"
		}), true
	case *ir.DbGetMine:
		tb := x.Table
		return f.dbCall("Db.getMine "+tb.Name, false, x.Ty, s, func(env string, c *scope) string {
			return "db.GetMineRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " + f.code(x.ID, c) + ")"
		}), true
	case *ir.DbInsertMine:
		tb := x.Table
		return f.dbCall("Db.insertMine "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			row := ""
			if r, ok := x.Row.(*ir.Record); ok { // always a literal (the checker): name its type by the alias
				row = f.record(r, insertMineName(tb.Module, tb.Name), c)
			} else {
				row = f.code(x.Row, c)
			}
			return "db.InsertMineRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " +
				insertMineVals(tb.Module, tb.Name) + "(" + row + "), " + env + ".Now())"
		}), true
	case *ir.DbUpdateMine:
		tb := x.Table
		return f.dbCall("Db.updateMine "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			return "struct{}{}, db.UpdateMineRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " +
				f.code(x.ID, c) + ", " + f.code(x.Fn, c) + ", " + env + ".Now())"
		}), true
	case *ir.DbDeleteMine:
		tb := x.Table
		return f.dbCall("Db.deleteMine "+tb.Name, true, x.Ty, s, func(env string, c *scope) string {
			return "struct{}{}, db.DeleteMineRow(" + dbArgs(env) + ", " + tableName(tb.Module, tb.Name) + ", " + f.code(x.ID, c) + ")"
		}), true
	case *ir.CapCall:
		cp := x.Cap
		key := cp.Module + "." + cp.Name
		step := cp.Name + "." + x.Entry
		return f.callStmts(step, !isQuery(x.Ty), x.Ty, s, func(env string, c *scope) string {
			// temporaries, so the arguments below still see any user local named like them
			impl, ok, v, err := f.temp(c, "_c"), f.temp(c, "_ok"), f.temp(c, "_v"), f.temp(c, "_err")
			args := []string{env + ".Ctx"}
			for _, a := range x.Args {
				args = append(args, f.code(a, c))
			}
			if len(x.Args) == 1 && capArgsWrapped(cp, x.Entry) { // Args__…{Value: v} (caps.go)
				args[1] = argsName(cp.Module, cp.Name, x.Entry) + "{Value: " + args[1] + "}"
			}
			return impl + ", " + ok + " := " + env + ".Caps[" + strconv.Quote(key) + "].(" + capName(cp.Module, cp.Name) + ")\n" +
				"if !" + ok + " {\npanic(" + strconv.Quote("alder: capability "+strconv.Quote(key)+" is not wired") + ")\n}\n" +
				v + ", " + err + " := " + impl + "." + Field(x.Entry) + "(" + strings.Join(args, ", ") + ")\n" +
				"return " + v + ", eff.CapError(" + strconv.Quote(step) + ", " + err + ")\n"
		}), true
	case *ir.TestRun:
		f.g.testNode(x)
		return f.testRun(x, s), true
	case *ir.TestTrace:
		f.g.testNode(x)
		return f.callStmts("Test.trace", false, x.Ty, s, func(env string, c *scope) string {
			return "return rtesting.Trace(" + env + "), nil\n"
		}), true
	case *ir.TestFake:
		f.g.testNode(x)
		cp := x.Fake.Capability
		key := strconv.Quote(cp.Module + "." + cp.Name)
		return f.callStmts("Test.fake "+cp.Name, false, x.Ty, s, func(env string, c *scope) string {
			return "if " + env + ".Caps == nil {\n" + env + ".Caps = map[string]any{}\n}\n" +
				env + ".Caps[" + key + "] = Fakes[" + key + "]()\nreturn struct{}{}, nil\n"
		}), true
	case *ir.TestLogin, *ir.TestGet, *ir.TestSubmit, *ir.TestSubmitRaw, *ir.TestClick:
		f.g.testNode(x)
		return f.browserStep(x, s), true // browser.go
	}
	panic(fmt.Sprintf("gen: %T is not an effect node", e))
}

// testNode panics — Emit's internal error — when test node x reaches the emitter in a
// production build, which emits no `test` bodies.
func (g *gen) testNode(x ir.Expr) {
	if !g.tests {
		panic(fmt.Sprintf("gen: %T in a production build (test nodes are emitted only with Options.Tests)", x))
	}
}

// testRun is Test.run: a step that runs the task inside the running Task under a savepoint
// (rtesting.Inner) and gives its outcome as the record { value: Result Text a, trace: List Text }.
func (f *fn) testRun(x *ir.TestRun, s *scope) string {
	r := GoType(effResult(x.Ty))
	a := GoType(effResult(x.Task.Type()))
	return f.callStmts("Test.run", true, x.Ty, s, func(env string, c *scope) string {
		task := f.task(x.Task, c)
		v, msg, ok, tr, err, out, zero := f.temp(c, "_v"), f.temp(c, "_msg"), f.temp(c, "_ok"), f.temp(c, "_tr"), f.temp(c, "_err"), f.temp(c, "_r"), f.temp(c, "_z")
		return v + ", " + msg + ", " + ok + ", " + tr + ", " + err + " := rtesting.Inner(" + env + ", " + task + ")\n" +
			"if " + err + " != nil {\nvar " + zero + " " + r + "\nreturn " + zero + ", " + err + "\n}\n" +
			out + " := " + r + "{Trace: " + tr + "}\n" +
			"if " + ok + " {\n" + out + ".Value = rt.Ok[string, " + a + "](" + v + ")\n} else {\n" + out + ".Value = rt.Err[string, " + a + "](" + msg + ")\n}\n" +
			"return " + out + ", nil\n"
	})
}

// bind is eff.Bind(effect, continuation): the continuation returns its body as a Task.
func (f *fn) bind(x *ir.EffBind, s *scope) string {
	eff := f.task(x.Effect, s)
	c := s.child()
	param := "_"
	var pt types.Type
	if x.Param != nil {
		param, pt = c.declare(x.Param.Name), x.Param.Type
	} else {
		pt = effResult(x.Effect.Type())
	}
	var w strings.Builder
	f.retK(&w, x.Body, c, f.task)
	return "eff.Bind(" + eff + ", func(" + param + " " + GoType(pt) + ") eff.Task[" + GoType(effResult(x.Ty)) + "] {\n" + w.String() + "})"
}

// dbArgs are the leading arguments of every rt/db operation inside a Call over env.
func dbArgs(env string) string {
	return env + ".Ctx, " + env + ".Store.(db.Store), db.TxOf(" + env + ")"
}

// dbCall is an eff.Call step named name whose function, over a fresh Env variable, returns
// body's result; t is the effect type of the step (its result type is the Call's).
func (f *fn) dbCall(name string, write bool, t types.Type, s *scope, body func(env string, c *scope) string) string {
	return f.callStmts(name, write, t, s, func(env string, c *scope) string { return "return " + body(env, c) + "\n" })
}

// callStmts is dbCall whose function body is the statements stmts writes.
func (f *fn) callStmts(name string, write bool, t types.Type, s *scope, stmts func(env string, c *scope) string) string {
	c := s.child()
	env := f.temp(c, "_e")
	return "eff.Call(" + strconv.Quote(name) + ", " + strconv.FormatBool(write) + ", func(" + env + " *eff.Env) (" +
		GoType(effResult(t)) + ", error) {\n" + stmts(env, c) + "})"
}

// run is the Call running a select (Db.all, Db.first, Db.count) of the table named table.
func (f *fn) run(table string, term ir.SelTerminal, t types.Type, s *scope, sel func(c *scope) string) string {
	ops := map[ir.SelTerminal][2]string{
		ir.SelAll: {"db.All", "Db.all"}, ir.SelFirst: {"db.First", "Db.first"}, ir.SelCount: {"db.CountRows", "Db.count"},
	}
	op, name := ops[term][0], ops[term][1]
	if op == "" {
		panic("gen: a select run with no terminal")
	}
	return f.dbCall(name+" "+table, false, t, s, func(env string, c *scope) string {
		return op + "(" + dbArgs(env) + ", " + sel(c) + ")"
	})
}

// selectTable is the (unqualified) name of the table of a `Select t` type.
func selectTable(t types.Type) string {
	if app, ok := t.(*types.App); ok && len(app.Args) == 1 {
		if r, ok := app.Args[0].(*types.Record); ok {
			return r.Name
		}
	}
	panic(fmt.Sprintf("gen: %s is not a Select of a table", types.String(t)))
}

// selectExpr is the db.Select a DbSelect pipeline builds (without its terminal).
func (f *fn) selectExpr(x *ir.DbSelect, s *scope) string {
	sel := "db.From(" + tableName(x.Table.Module, x.Table.Name) + ")"
	if x.Base != nil {
		sel = f.code(x.Base, s)
	}
	for _, w := range x.Where {
		sel = "db.Where(" + f.code(w, s) + ", " + sel + ")"
	}
	for _, o := range x.Order {
		sel = "db.OrderBy(" + f.code(o, s) + ", " + sel + ")"
	}
	if x.Limit != nil {
		sel = "db.Limit(" + f.code(x.Limit, s) + ", " + sel + ")"
	}
	if x.Offset != nil {
		sel = "db.Offset(" + f.code(x.Offset, s) + ", " + sel + ")"
	}
	return sel
}

var condFuncs = map[string]string{
	"eq": "db.CEq", "ne": "db.CNe", "lt": "db.CLt", "le": "db.CLe", "gt": "db.CGt", "ge": "db.CGe",
	"contains": "db.CContains", "startsWith": "db.CStartsWith", "isNothing": "db.CIsNull", "isJust": "db.CIsNotNull",
	"and": "db.CAnd", "or": "db.COr", "not": "db.CNot",
}

// cond is a condition as a db.Cond.
func (f *fn) cond(x *ir.Cond, s *scope) string {
	fn := condFuncs[x.Kind]
	switch x.Kind {
	case "and", "or":
		if x.Subs == nil && x.Value != nil {
			return fn + "(" + f.code(x.Value, s) + ")"
		}
		var cs []string
		for _, c := range x.Subs {
			cs = append(cs, f.code(c, s))
		}
		return fn + "([]db.Cond{" + strings.Join(cs, ", ") + "})"
	case "not":
		return fn + "(" + f.code(x.Subs[0], s) + ")"
	case "isNothing", "isJust":
		return fn + "(" + strconv.Quote(x.Col.Field) + ")"
	case "in":
		ct := columnType(x.Col)
		return "db.CIn(" + strconv.Quote(x.Col.Field) + ", db.EncodeList(" + f.code(x.Value, s) + ", " + encodeFunc(ct) + "))"
	}
	if fn == "" {
		panic("gen: unknown condition " + x.Kind)
	}
	// typed: an Int literal passed as `any` would be a Go int
	return fn + "(" + strconv.Quote(x.Col.Field) + ", " + encode(columnType(x.Col), f.typedExpr(x.Value, s)) + ")"
}

// columnType is the type of a column's field.
func columnType(c *ir.ColumnRef) types.Type {
	for _, fd := range c.Table.Record.Fields {
		if fd.Name == c.Field {
			return fd.Type
		}
	}
	panic(fmt.Sprintf("gen: table %s.%s has no column %s", c.Table.Module, c.Table.Name, c.Field))
}

// ---- column encoding (the storage mapping of rt/db)

// maybeArg is the a of `Maybe a`, or nil.
func maybeArg(t types.Type) types.Type {
	if u, ok := t.(*types.Union); ok && u.Module == "Prelude" && u.Name == "Maybe" && len(u.Args) == 1 {
		return u.Args[0]
	}
	return nil
}

// enumUnion is t as a payload-less union of the app (a CEnum column), or nil.
func enumUnion(t types.Type) *types.Union {
	if u, ok := t.(*types.Union); ok && u.Module != "Prelude" {
		return u
	}
	return nil
}

// encode is Go value v of column type t as the store keeps it.
func encode(t types.Type, v string) string {
	if a := maybeArg(t); a != nil {
		return "db.EncodeMaybe(" + v + ", " + encodeFunc(a) + ")"
	}
	if u := enumUnion(t); u != nil {
		return enumName(u.Module, u.Name) + "(" + v + ")"
	}
	switch GoType(t) {
	case "bool":
		return "db.EncodeBool(" + v + ")"
	case "rt.Date":
		return "db.EncodeDate(" + v + ")"
	case "time.Time":
		return "db.EncodeTime(" + v + ")"
	}
	return v
}

// encodeFunc is encode as a Go function of type func(T) any. Its parameter x shadows nothing it
// uses: its body refers to x alone.
func encodeFunc(t types.Type) string {
	return "func(x " + GoType(t) + ") any { return " + encode(t, "x") + " }"
}

// decodeFunc is a Go function of type func(any) (T, error) decoding a stored value of column
// type t (not a Maybe).
func decodeFunc(t types.Type) string {
	if u := enumUnion(t); u != nil {
		names, values := enumLists(u)
		return "func(v any) (" + GoType(t) + ", error) { return db.DecodeEnum(v, " + names + ", " + values + ") }"
	}
	switch GoType(t) {
	case "string":
		return "db.DecodeText"
	case "int64":
		return "db.DecodeInt"
	case "float64":
		return "db.DecodeFloat"
	case "bool":
		return "db.DecodeBool"
	case "rt.Date":
		return "db.DecodeDate"
	case "time.Time":
		return "db.DecodeTime"
	}
	panic(fmt.Sprintf("gen: type %s cannot be stored in a table", types.String(t)))
}

// enumLists are a payload-less union's constructor names and values, as Go slices.
func enumLists(u *types.Union) (names, values string) {
	var ns, vs []string
	for _, c := range u.Constructors() {
		ns = append(ns, strconv.Quote(c.Name))
		vs = append(vs, "{Tag: "+tagConst(u.Module, u.Name, c.Name)+"}")
	}
	return "[]string{" + strings.Join(ns, ", ") + "}", "[]" + GoType(u) + "{" + strings.Join(vs, ", ") + "}"
}

// columnKind is the db.Column fields Type, Nullable and Enum of a column of type t.
func columnKind(t types.Type) string {
	null := ""
	if a := maybeArg(t); a != nil {
		t, null = a, ", Nullable: true"
	}
	if u := enumUnion(t); u != nil {
		names, _ := enumLists(u)
		return "Type: db.CEnum" + null + ", Enum: " + names
	}
	k := map[string]string{"string": "db.CText", "int64": "db.CInt", "float64": "db.CFloat", "bool": "db.CBool",
		"rt.Date": "db.CDate", "time.Time": "db.CTime"}[GoType(t)]
	if k == "" {
		panic(fmt.Sprintf("gen: type %s cannot be stored in a table", types.String(t)))
	}
	return "Type: " + k + null
}

// ---- declarations: table descriptors, insert records, enum names

// tableDecl writes table tb's insert record type, its insert-values function and its descriptor.
// A table's fields are scalars or Maybe scalars (the checker's storable), so no field is boxed
// (types.go): a boxed field would be a pointer, which Encode/Decode below do not handle.
func (g *gen) tableDecl(w *strings.Builder, tb *ir.Table) {
	rec := GoName(tb.Module, tb.Name)
	for _, fd := range tb.Record.Fields {
		if g.boxed.has(tb.Module, tb.Name, fd.Name) {
			panic(fmt.Sprintf("gen: table field %s.%s.%s is boxed", tb.Module, tb.Name, fd.Name))
		}
	}
	field := func(name string) ir.FieldT {
		for _, fd := range tb.Record.Fields {
			if fd.Name == name {
				return fd
			}
		}
		panic(fmt.Sprintf("gen: table %s.%s has no field %s", tb.Module, tb.Name, name))
	}

	// the insert record: an alias of the anonymous struct Lower gives `Insert t`
	ins := insertName(tb.Module, tb.Name)
	var fs []string
	inserted := map[string]bool{}
	for _, n := range tb.InsertFields {
		fs = append(fs, Field(n)+" "+GoType(field(n).Type))
		inserted[n] = true
	}
	if len(fs) == 0 {
		fmt.Fprintf(w, "type %s = struct{}\n\n", ins)
	} else {
		fmt.Fprintf(w, "type %s = struct {\n%s\n}\n\n", ins, strings.Join(fs, "\n"))
	}
	var vals []string
	for _, fd := range tb.Record.Fields {
		if inserted[fd.Name] {
			vals = append(vals, encode(fd.Type, "r."+Field(fd.Name)))
		} else {
			vals = append(vals, "nil")
		}
	}
	r := "r"
	if len(tb.InsertFields) == 0 {
		r = "_"
	}
	fmt.Fprintf(w, "func %s(%s %s) []any {\nreturn []any{%s}\n}\n\n", insertVals(tb.Module, tb.Name), r, ins, strings.Join(vals, ", "))
	if tb.Owner != "" {
		g.insertMineDecl(w, tb, field)
	}

	// the descriptor
	fmt.Fprintf(w, "var %s = &db.Table[%s]{\nName: %s,\nColumns: []db.Column{\n", tableName(tb.Module, tb.Name), rec, strconv.Quote(tb.Module+"."+tb.Name))
	var enc []string
	var dec strings.Builder
	for i, fd := range tb.Record.Fields {
		fmt.Fprintf(w, "%s,\n", columnLit(tb, fd))
		enc = append(enc, encode(fd.Type, "r."+Field(fd.Name)))
		d := ""
		if a := maybeArg(fd.Type); a != nil {
			d = "db.DecodeMaybe(row[" + strconv.Itoa(i) + "], " + decodeFunc(a) + ")"
		} else if enumUnion(fd.Type) != nil {
			names, values := enumLists(enumUnion(fd.Type))
			d = "db.DecodeEnum(row[" + strconv.Itoa(i) + "], " + names + ", " + values + ")"
		} else {
			d = decodeFunc(fd.Type) + "(row[" + strconv.Itoa(i) + "])"
		}
		fmt.Fprintf(&dec, "if r.%s, err = %s; err != nil {\nreturn r, fmt.Errorf(\"column %s: %%w\", err)\n}\n", Field(fd.Name), d, fd.Name)
	}
	w.WriteString("},\n")
	fmt.Fprintf(w, "ID: %s,\n", strconv.Quote(tb.IDField))
	if tb.Created != "" {
		fmt.Fprintf(w, "Created: %s,\n", strconv.Quote(tb.Created))
	}
	if tb.Updated != "" {
		fmt.Fprintf(w, "Updated: %s,\n", strconv.Quote(tb.Updated))
	}
	n := len(tb.Record.Fields)
	fmt.Fprintf(w, "Encode: func(r %s) []any {\nreturn []any{%s}\n},\n", rec, strings.Join(enc, ", "))
	fmt.Fprintf(w, "Decode: func(row []any) (r %s, err error) {\nif len(row) != %d {\nreturn r, fmt.Errorf(\"%%d columns, want %d\", len(row))\n}\n%sreturn r, nil\n},\n",
		rec, n, n, dec.String())
	id := Field(tb.IDField)
	fmt.Fprintf(w, "ReadID: func(r %s) string { return r.%s },\n", rec, id)
	fmt.Fprintf(w, "SetID: func(r %s, id string) %s {\nr.%s = id\nreturn r\n},\n", rec, rec, id)
	if tb.Owner != "" {
		fmt.Fprintf(w, "Owner: %s,\nUser: %s,\n", strconv.Quote(tb.Owner), g.sessionUser())
	}
	w.WriteString("}\n\n")
}

// insertMineDecl writes table tb's Db.insertMine row type, insertmine__M__T (an alias of the
// anonymous struct Lower gives the row: the InsertFields without the owner column), and its
// column values function, insertminevals__M__T, which leaves the owner column nil for
// db.InsertMineRow to fill with the signed-in user.
func (g *gen) insertMineDecl(w *strings.Builder, tb *ir.Table, field func(string) ir.FieldT) {
	var fs []string
	inserted := map[string]bool{}
	for _, n := range tb.InsertFields {
		if n != tb.Owner {
			fs = append(fs, Field(n)+" "+GoType(field(n).Type))
			inserted[n] = true
		}
	}
	ins := insertMineName(tb.Module, tb.Name)
	if len(fs) == 0 {
		fmt.Fprintf(w, "type %s = struct{}\n\n", ins)
	} else {
		fmt.Fprintf(w, "type %s = struct {\n%s\n}\n\n", ins, strings.Join(fs, "\n"))
	}
	var vals []string
	for _, fd := range tb.Record.Fields {
		if inserted[fd.Name] {
			vals = append(vals, encode(fd.Type, "r."+Field(fd.Name)))
		} else {
			vals = append(vals, "nil")
		}
	}
	r := "r"
	if len(fs) == 0 {
		r = "_"
	}
	fmt.Fprintf(w, "func %s(%s %s) []any {\nreturn []any{%s}\n}\n\n", insertMineVals(tb.Module, tb.Name), r, ins, strings.Join(vals, ", "))
}

// sessionUser is the descriptor's User function of a table with an `@owner` column: the
// signed-in user of App's session value s (the running effect's Env.Session), false when s is
// no App session or its `user` is Nothing.
func (g *gen) sessionUser() string {
	if g.session == nil {
		return "func(any) (string, bool) { return \"\", false }"
	}
	app := GoName(g.session.Module, g.session.Name)
	for _, fd := range g.session.Fields {
		if fd.Name != "user" {
			continue
		}
		if maybeArg(fd.Type) != nil {
			return "func(s any) (string, bool) {\na, ok := s.(" + app + ")\nreturn a.User.Val, ok && a.User.Ok\n}"
		}
		return "func(s any) (string, bool) {\na, ok := s.(" + app + ")\nreturn a.User, ok\n}"
	}
	return "func(any) (string, bool) { return \"\", false }"
}

// columnLit is the db.Column literal of table tb's field fd.
func columnLit(tb *ir.Table, fd ir.FieldT) string {
	col := "{Name: " + strconv.Quote(fd.Name) + ", " + columnKind(fd.Type)
	if slices.Contains(tb.Unique, fd.Name) {
		col += ", Unique: true"
	}
	if ref := tb.Refs[fd.Name]; ref != "" {
		col += ", Ref: " + strconv.Quote(ref)
		switch tb.OnDelete[fd.Name] { // @restrict, the default, is db.Restrict, the zero value
		case "cascade":
			col += ", OnDelete: db.Cascade"
		case "setNull":
			col += ", OnDelete: db.SetNull"
		}
	}
	return col + "}"
}

// enumDecl writes the function naming a value of payload-less union u (stored in a table).
func enumDecl(w *strings.Builder, u *ir.UnionType) {
	decl, use := unionParams(u)
	var ns []string
	for _, c := range u.Ctors {
		ns = append(ns, strconv.Quote(c.Name))
	}
	fmt.Fprintf(w, "func %s%s(v %s%s) string {\nreturn [...]string{%s}[v.Tag]\n}\n\n",
		enumName(u.Module, u.Name), decl, GoName(u.Module, u.Name), use, strings.Join(ns, ", "))
}

// storedEnums are the unions ("Module.Name") some table column holds (directly or in a Maybe):
// they get an enumDecl.
func storedEnums(p *ir.Program) map[string]bool {
	out := map[string]bool{}
	for _, tb := range p.Tables {
		for _, fd := range tb.Record.Fields {
			t := fd.Type
			if a := maybeArg(t); a != nil {
				t = a
			}
			if u := enumUnion(t); u != nil {
				out[u.Module+"."+u.Name] = true
			}
		}
	}
	return out
}
