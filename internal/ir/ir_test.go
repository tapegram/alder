package ir

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

// lowerDir loads, resolves and checks dir, requires it to be free of errors, and lowers it.
func lowerDir(t *testing.T, dir string) *Program {
	t.Helper()
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	if prog.ParseFailed {
		t.Fatalf("%s does not parse:\n%s", dir, errors(bag))
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		t.Fatalf("%s does not check:\n%s", dir, errors(bag))
	}
	return Lower(prog, res)
}

func errors(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity == "error" {
			fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		}
	}
	return sb.String()
}

// compareGolden checks got against the golden file, or rewrites it under -update.
func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update)", golden)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

func TestIRGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/ir_*")
	n := 0
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			compareGolden(t, dir+".ir", Dump(lowerDir(t, dir)))
		})
	}
	if n == 0 {
		t.Fatal("no IR golden directories")
	}
}

// TestIR_Todos lowers the real example app: it must not panic (a missing recorded type is a
// checker bug), and its dump is reviewed by hand in testdata/golden/ir_todos.ir.
func TestIR_Todos(t *testing.T) {
	p := lowerDir(t, "../../examples/todos")
	got := Dump(p)
	compareGolden(t, "../../testdata/golden/ir_todos.ir", got)
	for _, want := range []string{"(value Todos.Index.view ", "(component Todos.Index.Item ", "(component Layout.Page ",
		"(skipped-effect Todos.Index.load)", "(skipped-effect Todos.Index.update)", "(skipped-effect App.update)"} {
		if !strings.Contains(got, want) {
			t.Errorf("todos dump lacks %q", want)
		}
	}
	for _, pg := range p.Pages {
		if pg.View == nil || pg.Load != nil || pg.Update != nil {
			t.Errorf("page %s: view %v, load %v, update %v; want only a view in M3a", pg.Module, pg.View != nil, pg.Load != nil, pg.Update != nil)
		}
	}
}

// TestIR_NoBrands walks every node, type and map of the lowered todos app and of every IR golden
// (brands appear in tables, messages, page parameters, the session and values) and requires that
// none survives.
func TestIR_NoBrands(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/ir_*")
	dirs = append(dirs, "../../examples/todos")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) { noBrands(t, lowerDir(t, dir)) })
	}
	if !reflect.TypeFor[*types.Brand]().Implements(reflect.TypeFor[types.Type]()) {
		t.Fatal("types.Brand is no longer a types.Type; update this test")
	}
}

func noBrands(t *testing.T, p *Program) {
	seen := map[uintptr]bool{}
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() || seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			if _, isBrand := v.Interface().(*types.Brand); isBrand {
				t.Errorf("brand left in the IR at %s", path)
				return
			}
			walk(v.Elem(), path)
		case reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), path)
			}
		case reflect.Struct:
			for i := range v.NumField() {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i), path+"."+v.Type().Field(i).Name)
				}
			}
		case reflect.Slice:
			for i := range v.Len() {
				walk(v.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(v.MapIndex(k), fmt.Sprintf("%s[%v]", path, k))
			}
		}
	}
	walk(reflect.ValueOf(p), "Program")
}

// TestIR_CtorPayloadShared: a record literal given to a constructor, directly or through `|>`,
// is a Record of the constructor's own payload RecordType, and a message payload omits `form`.
func TestIR_CtorPayloadShared(t *testing.T) {
	p := lowerDir(t, "../../testdata/golden/ir_patterns")
	var boxed *Value
	for _, v := range p.Modules[0].Values {
		if v.Name == "boxed" || v.Name == "nodeOf" {
			c := v.Body.(*Ctor)
			rec, ok := c.Payload.(*Record)
			if !ok || rec.T != c.U.ctorT(c.Name).Payload {
				t.Errorf("%s: payload is %T with RecordType %p, want the ctor's %p", v.Name, c.Payload, rec.T, c.U.ctorT(c.Name).Payload)
			}
			if v.Name == "boxed" {
				boxed = v
			}
		}
	}
	if boxed == nil {
		t.Fatal("ir_patterns has no `boxed`")
	}
	pg := lowerDir(t, "../../testdata/golden/ir_page")
	var save *Record
	for _, m := range pg.Modules {
		for _, v := range m.Values {
			if v.Module == "Edit" && v.Name == "view" {
				form := v.Body.(*Elem).Children[2].(*FormEl)
				save = form.Attrs[0].Value.(*Ctor).Payload.(*Record)
			}
		}
	}
	if save == nil || len(save.Fields) != 1 || save.Fields[0].Name != "n" || save.T != pg.Pages[0].Msgs[0].Params {
		t.Fatalf("Save payload: %+v; want only field n, of the message's Params record", save)
	}
}
