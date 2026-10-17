// internal/syntax/dump.go
package syntax

import (
	"fmt"
	"strings"
)

// Dump renders any node as a compact S-expression. Used by golden tests.
func Dump(n any) string {
	var sb strings.Builder
	dump(&sb, n)
	return sb.String()
}

func dumpList[T any](sb *strings.Builder, items []T) {
	for _, it := range items {
		sb.WriteByte(' ')
		dump(sb, it)
	}
}

func dump(sb *strings.Builder, n any) {
	switch x := n.(type) {
	case nil:
		sb.WriteString("nil")
	case *Module:
		fmt.Fprintf(sb, "(module %s", x.Name)
		for _, im := range x.Imports {
			fmt.Fprintf(sb, " (import %s)", im.Module)
		}
		dumpList(sb, x.Decls)
		sb.WriteByte(')')
	case *ValueDecl:
		fmt.Fprintf(sb, "(val %s", x.Name)
		if x.Sig != nil {
			sb.WriteString(" : ")
			dump(sb, x.Sig)
		}
		for _, p := range x.Params {
			sb.WriteString(" " + p.Name)
		}
		if x.Body != nil {
			sb.WriteString(" = ")
			dump(sb, x.Body)
		}
		sb.WriteByte(')')
	case *TypeDecl:
		fmt.Fprintf(sb, "(type %s", x.Name)
		for _, p := range x.Params {
			sb.WriteString(" " + p)
		}
		switch {
		case x.Union != nil:
			for _, c := range x.Union {
				fmt.Fprintf(sb, " (ctor %s", c.Name)
				if c.Arg != nil {
					sb.WriteByte(' ')
					dump(sb, c.Arg)
				}
				sb.WriteByte(')')
			}
		case x.Record != nil:
			sb.WriteByte(' ')
			dump(sb, x.Record)
		case x.Brand != nil:
			fmt.Fprintf(sb, " (brand %s", x.Brand.Base)
			if x.Brand.Check != nil {
				sb.WriteString(" where ")
				dump(sb, x.Brand.Check)
			}
			sb.WriteByte(')')
		default:
			sb.WriteString(" opaque")
		}
		sb.WriteByte(')')
	case *PageDecl:
		fmt.Fprintf(sb, "(page %q ", x.Path)
		dump(sb, x.Params)
		if x.Requires != "" {
			sb.WriteString(" requires " + x.Requires)
		}
		if len(x.Roles) > 0 {
			sb.WriteString(" role " + strings.Join(x.Roles, " | "))
		}
		sb.WriteByte(')')
	case *MsgDecl:
		fmt.Fprintf(sb, "(msg %s", x.Name)
		if x.Params != nil {
			sb.WriteByte(' ')
			dump(sb, x.Params)
		}
		if x.Form != "" {
			sb.WriteString(" form " + x.Form)
		}
		sb.WriteByte(')')
	case *FormDecl:
		fmt.Fprintf(sb, "(form %s", x.Name)
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *TableDecl:
		fmt.Fprintf(sb, "(table %s", x.Name)
		for _, a := range x.Attrs {
			fmt.Fprintf(sb, " @%s %q", a.Name, a.Arg)
		}
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *CapDecl:
		fmt.Fprintf(sb, "(capability %s", x.Name)
		dumpList(sb, x.Entries)
		sb.WriteByte(')')
	case *FakeDecl:
		fmt.Fprintf(sb, "(fake %s", x.Name)
		for _, t := range x.Tables {
			sb.WriteByte(' ')
			dump(sb, t)
		}
		for _, e := range x.Entries {
			sb.WriteByte(' ')
			dump(sb, e)
		}
		sb.WriteByte(')')
	case *SessionDecl:
		sb.WriteString("(session")
		dumpList(sb, x.Fields)
		sb.WriteByte(')')
	case *DeployDecl:
		sb.WriteString("(deploy")
		for _, f := range x.Fields {
			sb.WriteByte(' ')
			dumpDeployField(sb, f)
		}
		sb.WriteByte(')')
	case *LoginDecl:
		fmt.Fprintf(sb, "(login %s)", x.Page)
	case *TestDecl:
		fmt.Fprintf(sb, "(test %q ", x.Name)
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *Field:
		fmt.Fprintf(sb, "(field %s ", x.Name)
		dump(sb, x.Type)
		if x.Default != nil {
			sb.WriteString(" = ")
			dump(sb, x.Default)
		}
		for _, a := range x.Attrs {
			fmt.Fprintf(sb, " @%s", a.Name)
			if a.Name == "renamed" {
				fmt.Fprintf(sb, " %q", a.Arg)
			} else if a.Arg != "" {
				sb.WriteString(" " + a.Arg)
			}
		}
		sb.WriteByte(')')
	case *TypeName:
		if len(x.Args) == 0 {
			sb.WriteString(qual(x.Module, x.Name))
			return
		}
		fmt.Fprintf(sb, "(%s", qual(x.Module, x.Name))
		dumpList(sb, x.Args)
		sb.WriteByte(')')
	case *TypeVar:
		sb.WriteString(x.Name)
	case *RecordType:
		sb.WriteString("{")
		for i, f := range x.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f.Name + ": ")
			dump(sb, f.Type)
			if f.Default != nil {
				sb.WriteString(" = ")
				dump(sb, f.Default)
			}
		}
		sb.WriteString("}")
	case *FunType:
		sb.WriteString("(fn")
		dumpList(sb, x.Params)
		sb.WriteString(" -> ")
		dump(sb, x.Result)
		sb.WriteByte(')')
	case *IntLit:
		fmt.Fprintf(sb, "(int %d)", x.Value)
	case *FloatLit:
		fmt.Fprintf(sb, "(float %g)", x.Value)
	case *BoolLit:
		fmt.Fprintf(sb, "(bool %v)", x.Value)
	case *UnitLit:
		sb.WriteString("(unit)")
	case *StrLit:
		sb.WriteString("(str")
		for _, p := range x.Parts {
			if p.Expr == nil {
				fmt.Fprintf(sb, " %q", p.Text)
			} else {
				sb.WriteByte(' ')
				dump(sb, p.Expr)
			}
		}
		sb.WriteByte(')')
	case *ListLit:
		sb.WriteString("(list")
		dumpList(sb, x.Elems)
		sb.WriteByte(')')
	case *RecordLit:
		sb.WriteString("(record")
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Value)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *RecordUpdate:
		sb.WriteString("(update ")
		dump(sb, x.Base)
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Value)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *Var:
		fmt.Fprintf(sb, "(var %s)", qual(x.Module, x.Name))
	case *CtorRef:
		fmt.Fprintf(sb, "(ctor %s)", qual(x.Module, x.Name))
	case *Access:
		sb.WriteString("(access ")
		dump(sb, x.Expr)
		sb.WriteString(" " + x.Field + ")")
	case *App:
		sb.WriteString("(app ")
		dump(sb, x.Fn)
		dumpList(sb, x.Args)
		sb.WriteByte(')')
	case *Lambda:
		sb.WriteString("(lambda")
		for _, p := range x.Params {
			sb.WriteString(" " + p.Name)
		}
		sb.WriteString(" -> ")
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *If:
		sb.WriteString("(if ")
		dump(sb, x.Cond)
		sb.WriteByte(' ')
		dump(sb, x.Then)
		sb.WriteByte(' ')
		dump(sb, x.Else)
		sb.WriteByte(')')
	case *Match:
		sb.WriteString("(match ")
		dump(sb, x.Subject)
		dumpList(sb, x.Arms)
		sb.WriteByte(')')
	case *Arm:
		sb.WriteString("(arm ")
		dump(sb, x.Pat)
		sb.WriteByte(' ')
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *Block:
		sb.WriteString("(block")
		dumpList(sb, x.Stmts)
		sb.WriteByte(' ')
		dump(sb, x.Result)
		sb.WriteByte(')')
	case *LetStmt:
		op := "="
		if x.Bind {
			op = "<-"
		}
		fmt.Fprintf(sb, "(let %s", x.Name)
		if x.Type != nil {
			sb.WriteString(" : ")
			dump(sb, x.Type)
		}
		sb.WriteString(" " + op + " ")
		dump(sb, x.Expr)
		sb.WriteByte(')')
	case *BinOp:
		fmt.Fprintf(sb, "(%s ", x.Op)
		dump(sb, x.L)
		sb.WriteByte(' ')
		dump(sb, x.R)
		sb.WriteByte(')')
	case *HoleExpr:
		fmt.Fprintf(sb, "(hole %s)", x.Name)
	case *Element:
		fmt.Fprintf(sb, "(el %s", qual(x.Module, x.Tag))
		for _, a := range x.Attrs {
			fmt.Fprintf(sb, " (attr %s", a.Name)
			if a.Value != nil {
				sb.WriteByte(' ')
				dump(sb, a.Value)
			}
			sb.WriteByte(')')
		}
		dumpList(sb, x.Children)
		sb.WriteByte(')')
	case *HtmlText:
		fmt.Fprintf(sb, "(text %q)", x.Text)
	case *HtmlExpr:
		sb.WriteString("(expr ")
		dump(sb, x.Expr)
		sb.WriteByte(')')
	case *HtmlFor:
		fmt.Fprintf(sb, "(for %s ", x.Var)
		dump(sb, x.List)
		sb.WriteByte(' ')
		dump(sb, x.Body)
		sb.WriteByte(')')
	case *HtmlIf:
		sb.WriteString("(hif ")
		dump(sb, x.Cond)
		sb.WriteByte(' ')
		dump(sb, x.Then)
		sb.WriteByte(' ')
		dump(sb, x.Else)
		sb.WriteByte(')')
	case *HtmlMatch:
		sb.WriteString("(hmatch ")
		dump(sb, x.Subject)
		dumpList(sb, x.Arms)
		sb.WriteByte(')')
	case *PWild:
		sb.WriteString("_")
	case *PVar:
		sb.WriteString(x.Name)
	case *PLit:
		dump(sb, x.Lit)
	case *PCtor:
		if x.Arg == nil {
			fmt.Fprintf(sb, "(pctor %s)", x.Name)
			return
		}
		fmt.Fprintf(sb, "(pctor %s ", x.Name)
		dump(sb, x.Arg)
		sb.WriteByte(')')
	case *PRecord:
		sb.WriteString("(prec")
		for _, f := range x.Fields {
			fmt.Fprintf(sb, " (%s ", f.Name)
			dump(sb, f.Pat)
			sb.WriteByte(')')
		}
		sb.WriteByte(')')
	case *PList:
		sb.WriteString("(plist")
		dumpList(sb, x.Elems)
		if x.Rest != "" {
			sb.WriteString(" .." + x.Rest)
		}
		sb.WriteByte(')')
	default:
		fmt.Fprintf(sb, "<%T>", n)
	}
}

func qual(m, n string) string {
	if m == "" {
		return n
	}
	return m + "." + n
}

func dumpDeployField(sb *strings.Builder, f *DeployField) {
	fmt.Fprintf(sb, "(%s ", f.Name)
	dumpDeployValue(sb, f.Value)
	sb.WriteByte(')')
}

func dumpDeployValue(sb *strings.Builder, v *DeployValue) {
	switch v.Kind {
	case "string":
		fmt.Fprintf(sb, "%q", v.Str)
	case "list":
		sb.WriteByte('[')
		for i, e := range v.List {
			if i > 0 {
				sb.WriteByte(' ')
			}
			dumpDeployValue(sb, e)
		}
		sb.WriteByte(']')
	case "ctor":
		sb.WriteString(v.Ctor)
		if v.Arg != nil {
			sb.WriteByte(' ')
			dumpDeployValue(sb, v.Arg)
		}
		if v.Rec != nil {
			sb.WriteString(" {")
			for i, f := range v.Rec {
				if i > 0 {
					sb.WriteByte(' ')
				}
				dumpDeployField(sb, f)
			}
			sb.WriteByte('}')
		}
	default:
		sb.WriteString(v.Str)
	}
}
