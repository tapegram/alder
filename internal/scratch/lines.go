package scratch

import (
	"fmt"
	"strings"

	"alder/internal/diag"
)

// Runnable reports whether the build produced an executable. A program with an error or a
// typed hole leaves Binary "" with Err nil (the reason is in Diags); a failed vet or go build
// leaves it "" with Err set.
func (b Build) Runnable() bool { return b.Binary != "" }

// SourceLine maps line l of the extra module as Compile rewrote it to the line of the caller's
// Module.Source: lines before the inserted ones are unchanged, lines after them move up by
// LineOffset, and an inserted line (a synthesized `module` line or an implicit import) has no
// source line (ok false).
func (b Build) SourceLine(l int) (line int, ok bool) {
	switch {
	case b.LineOffset == 0 || l < b.InsertLine:
		return l, true
	case l < b.InsertLine+b.LineOffset:
		return 0, false
	default:
		return l - b.LineOffset, true
	}
}

// MapDiag maps a diagnostic of the extra module (Span.File == b.ExtraFile) to the caller's
// source: its span's lines and its fix's lines go through SourceLine. A fix that touches an
// inserted line is dropped; a diagnostic whose span does is returned with ok false (it is about
// text the caller did not write). Other diagnostics are returned unchanged. Callers should
// report the extra module's diagnostics through MapDiag rather than subtracting LineOffset.
func (b Build) MapDiag(d diag.Diagnostic) (diag.Diagnostic, bool) {
	if b.ExtraFile == "" || d.Span.File != b.ExtraFile {
		return d, true
	}
	start, ok1 := b.SourceLine(d.Span.Start.Line)
	end, ok2 := b.SourceLine(d.Span.End.Line)
	if !ok1 || !ok2 {
		return d, false
	}
	d.Span.Start.Line, d.Span.End.Line = start, end
	if d.Fix != nil {
		l, ok1 := b.SourceLine(d.Fix.Line)
		e, ok2 := b.SourceLine(d.Fix.EndLine)
		if ok1 && ok2 {
			f := *d.Fix
			f.Line, f.EndLine = l, e
			d.Fix = &f
		} else {
			d.Fix = nil
		}
	}
	return d, true
}

// headerLines classifies the header of src, the lines before its first declaration (blank
// lines and `--` comments do not end it): it returns the index of the `module` line (-1 for
// none), the module name declared there, and the modules imported in the header.
func headerLines(src string) (modLine int, declared string, imports []string) {
	modLine = -1
	for i, l := range strings.Split(src, "\n") {
		f := strings.Fields(l)
		switch {
		case len(f) == 0 || strings.HasPrefix(f[0], "--"):
			continue
		case f[0] == "module" && modLine < 0 && len(imports) == 0:
			modLine = i
			if len(f) > 1 {
				declared = f[1]
			}
		case f[0] == "import" && len(f) > 1:
			imports = append(imports, f[1])
		default:
			return modLine, declared, imports
		}
	}
	return modLine, declared, imports
}

// checkDeclaredName reports a `module` line naming a module other than name: the generated
// names and the diagnostics' file would disagree.
func checkDeclaredName(src, name string) error {
	if _, declared, _ := headerLines(src); declared != "" && declared != name {
		return fmt.Errorf("the module's source declares `module %s`, but it is compiled as %s; use the same name", declared, name)
	}
	return nil
}

// insertInfo is the InsertLine and ExtraFile of the extra module name with source src, into
// which withImports inserted offset lines.
func insertInfo(src, name string, offset int) (insertLine int, file string) {
	if offset > 0 {
		modLine, _, _ := headerLines(src)
		insertLine = modLine + 2 // the line after the module line; 1 when synthesized (-1 + 2)
	}
	return insertLine, name + ".alder"
}
