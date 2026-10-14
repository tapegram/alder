package generate

import (
	"fmt"
	"strings"
	"unicode"

	"alder/internal/syntax"
)

// The field syntax of the generators' command lines: one argument per field,
//
//	name:Type[=default][@attr[=arg]]…
//
// such as `title:Text.NonEmpty`, `done:Bool=False`, `post:Post.PostId@ref=Post@cascade`,
// `owner:User.UserId@ref=User@owner`, `"note:Maybe Text"` (a type with spaces is quoted for
// the shell), `slug:Post.Slug@unique` and `body:Text@renamed=content`. The type is written as
// it is written in the module the field goes into. The default is everything between the
// first `=` after the type and the first `@` outside a string literal. An attribute's argument
// follows `=`: `@ref=T` is `@ref T`, `@renamed=old` is `@renamed "old"`.

// FieldSpec is one parsed field.
type FieldSpec struct {
	Name    string
	Type    string // as written, spaces normalised: "Maybe Text"
	Default string // "" for none: "False", "\"x\"", "Reader"
	Attrs   []AttrSpec
}

// AttrSpec is one attribute of a field: Name without the `@`, Arg its argument ("" for none).
type AttrSpec struct{ Name, Arg string }

// knownAttrs are the table field attributes (chapter 8), with whether each takes an argument.
var knownAttrs = map[string]bool{
	"id": false, "ref": true, "created": false, "updated": false, "unique": false,
	"cascade": false, "restrict": false, "setNull": false, "renamed": true, "owner": false,
}

// ParseField parses one field argument. table says whether attributes are allowed (a form's
// fields take none).
func ParseField(arg string, table bool) (FieldSpec, error) {
	var f FieldSpec
	name, rest, ok := strings.Cut(arg, ":")
	if !ok {
		return f, fmt.Errorf("`%s` is not a field: write `name:Type`, such as `title:Text.NonEmpty`", arg)
	}
	name = strings.TrimSpace(name)
	if !IsLowerIdent(name) {
		return f, fmt.Errorf("`%s` is not a field name: a field name is a lower-case identifier, such as `title`", name)
	}
	f.Name = name
	// split off the attributes: the first `@` outside a string literal
	head, attrs := rest, ""
	inStr := false
	for i, r := range rest {
		switch {
		case r == '"':
			inStr = !inStr
		case r == '@' && !inStr:
			head, attrs = rest[:i], rest[i:]
		}
		if attrs != "" {
			break
		}
	}
	typ, def, hasDef := strings.Cut(head, "=")
	f.Type = strings.Join(strings.Fields(typ), " ")
	if f.Type == "" {
		return f, fmt.Errorf("field `%s` has no type: write `%s:Type`", name, name)
	}
	for _, r := range f.Type {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == ' ' || r == '(' || r == ')' || r == '_') {
			return f, fmt.Errorf("field `%s`: `%s` is not a type", name, f.Type)
		}
	}
	if hasDef {
		f.Default = strings.TrimSpace(def)
		if f.Default == "" {
			return f, fmt.Errorf("field `%s` has an empty default: write `%s:%s=value` or leave out the `=`", name, name, f.Type)
		}
		if strings.Contains(f.Default, "--") {
			return f, fmt.Errorf("field `%s`: the default `%s` holds `--`, which begins a comment in Alder source: write the value without it", name, f.Default)
		}
		if _, ds, more, _ := syntax.ParseExpr("default", []byte(f.Default)); len(ds) > 0 || more != "" || strings.Contains(f.Default, "\n") {
			return f, fmt.Errorf("field `%s`: the default `%s` is not one expression", name, f.Default)
		}
	}
	if attrs != "" {
		if !table {
			return f, fmt.Errorf("field `%s`: a form field takes no attributes (`%s`)", name, attrs)
		}
		for _, a := range strings.Split(attrs[1:], "@") {
			an, arg, hasArg := strings.Cut(strings.TrimSpace(a), "=")
			takes, known := knownAttrs[an]
			switch {
			case !known:
				return f, fmt.Errorf("field `%s`: `@%s` is not an attribute (they are @id, @ref=Table, @created, @updated, @unique, @cascade, @restrict, @setNull, @owner, @renamed=old)", name, an)
			case takes && (!hasArg || arg == ""):
				return f, fmt.Errorf("field `%s`: `@%s` takes an argument: `@%s=%s`", name, an, an, map[string]string{"ref": "Table", "renamed": "old"}[an])
			case !takes && hasArg:
				return f, fmt.Errorf("field `%s`: `@%s` takes no argument", name, an)
			case an == "ref" && !IsUpperIdent(arg):
				return f, fmt.Errorf("field `%s`: `@ref=%s`: a table is named by a capitalised identifier, such as `@ref=Post`", name, arg)
			case an == "renamed" && !IsLowerIdent(arg) && !IsUpperIdent(arg):
				return f, fmt.Errorf("field `%s`: `@renamed=%s`: the old name is an identifier, such as `@renamed=body`", name, arg)
			}
			f.Attrs = append(f.Attrs, AttrSpec{an, arg})
		}
	}
	return f, nil
}

// Has reports whether the field has attribute name.
func (f FieldSpec) Has(name string) bool {
	for _, a := range f.Attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

// Source is the field as declared in a table or a form: `name: Type = default @attr …`.
func (f FieldSpec) Source() string {
	s := f.Name + ": " + f.Type
	if f.Default != "" {
		s += " = " + f.Default
	}
	for _, a := range f.Attrs {
		s += " " + a.Source()
	}
	return s
}

// Source is the attribute as written in a table: `@ref Post`, `@renamed "old"`, `@unique`.
func (a AttrSpec) Source() string {
	switch a.Name {
	case "ref":
		return "@ref " + a.Arg
	case "renamed":
		return "@renamed " + quote(a.Arg)
	}
	return "@" + a.Name
}

// IsLowerIdent reports whether s is a lower-case identifier: a lower-case letter, then letters
// and digits.
func IsLowerIdent(s string) bool {
	for i, r := range s {
		switch {
		case i == 0 && !(r >= 'a' && r <= 'z'):
			return false
		case !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'):
			return false
		}
	}
	return s != "" && !keywords[s]
}

// IsUpperIdent reports whether s is a capitalised identifier: an upper-case letter, then
// letters and digits.
func IsUpperIdent(s string) bool {
	for i, r := range s {
		switch {
		case i == 0 && !(r >= 'A' && r <= 'Z'):
			return false
		case !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'):
			return false
		}
	}
	return s != ""
}

// IsModuleName reports whether s is a module name: capitalised identifiers joined by dots.
func IsModuleName(s string) bool {
	for _, seg := range strings.Split(s, ".") {
		if !IsUpperIdent(seg) {
			return false
		}
	}
	return true
}

// keywords are the lower-case words a field or value may not be named.
var keywords = map[string]bool{
	"module": true, "import": true, "type": true, "brand": true, "where": true, "table": true,
	"form": true, "msg": true, "page": true, "requires": true, "session": true, "capability": true,
	"fake": true, "test": true, "let": true, "if": true, "then": true, "else": true, "match": true,
	"for": true, "in": true, "true": true, "false": true,
}

// quote is s as an Alder string literal (no interpolation: `{` is escaped).
func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "{", `\{`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}
