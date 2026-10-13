// Package elements reads Custom Elements Manifests (schema 1.0.0): the custom elements an Alder
// program may use, with their attributes and slots. The checker unites two of them: the
// `alder-ui` design system's, embedded with the runtime (rt/al/custom-elements.json, Builtin),
// and the app's own at elements/custom-elements.json (Parse), which declares third-party
// elements and wins over the built-in one for a tag both declare (Merge).
//
// Only the part of the schema the checker needs is read: each module's declarations with a
// tagName, their attributes (name, type.text, default, description) and their slots (name).
// Everything else (CSS parts, events, members, paths) is ignored.
package elements

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"sync"

	"alder/rt"
)

// Path is where an app declares its custom elements, relative to the app's root.
const Path = "elements/custom-elements.json"

// builtinPath is the embedded design system's manifest inside rt.Files.
const builtinPath = "al/custom-elements.json"

// Manifest is the custom elements of one manifest, or of several merged, by tag.
type Manifest struct {
	Elements map[string]*Element
}

// Element is one custom element's declaration.
type Element struct {
	Tag     string
	Attrs   []Attr   // in manifest order
	Slots   []string // in manifest order; "" is the default slot
	Builtin bool     // declared by the embedded `alder-ui` manifest (not overridden by the app's)
}

// Attr is one declared attribute.
type Attr struct {
	Name, Type, Default, Description string
}

// Boolean reports whether a takes no value (its type is `boolean`).
func (a Attr) Boolean() bool { return strings.TrimSpace(a.Type) == "boolean" }

// Attr is e's attribute name, or nil.
func (e *Element) Attr(name string) *Attr {
	for i := range e.Attrs {
		if e.Attrs[i].Name == name {
			return &e.Attrs[i]
		}
	}
	return nil
}

// HasSlot reports whether e declares the slot name ("" for the default slot).
func (e *Element) HasSlot(name string) bool {
	for _, s := range e.Slots {
		if s == name {
			return true
		}
	}
	return false
}

// NamedSlots are e's slots other than the default one, in manifest order.
func (e *Element) NamedSlots() []string {
	var out []string
	for _, s := range e.Slots {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Lookup is the element tag, or nil. m may be nil.
func (m *Manifest) Lookup(tag string) *Element {
	if m == nil {
		return nil
	}
	return m.Elements[tag]
}

// Error is a manifest that does not parse: Reason says why ("no `modules`", or the JSON
// decoder's complaint), at Line and Col of the file (1-based; 1:1 when there is no place).
type Error struct {
	Line, Col int
	Reason    string
}

func (e *Error) Error() string { return e.Reason }

type cemFile struct {
	Modules *[]struct {
		Declarations []struct {
			TagName    string `json:"tagName"`
			Attributes []struct {
				Name        string                `json:"name"`
				Type        struct{ Text string } `json:"type"`
				Default     string                `json:"default"`
				Description string                `json:"description"`
			} `json:"attributes"`
			Slots []struct {
				Name string `json:"name"`
			} `json:"slots"`
		} `json:"declarations"`
	} `json:"modules"`
}

// Parse reads a manifest. A declaration without a tagName is not a custom element and is
// skipped; a tag declared twice keeps its last declaration.
func Parse(data []byte) (*Manifest, error) {
	var f cemFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, jsonError(data, err)
	}
	if f.Modules == nil {
		return nil, &Error{Line: 1, Col: 1, Reason: "no `modules`"}
	}
	m := &Manifest{Elements: map[string]*Element{}}
	for _, mod := range *f.Modules {
		for _, d := range mod.Declarations {
			if d.TagName == "" {
				continue
			}
			el := &Element{Tag: d.TagName}
			for _, a := range d.Attributes {
				if a.Name != "" {
					el.Attrs = append(el.Attrs, Attr{Name: a.Name, Type: a.Type.Text, Default: a.Default, Description: a.Description})
				}
			}
			for _, s := range d.Slots {
				el.Slots = append(el.Slots, s.Name)
			}
			m.Elements[el.Tag] = el
		}
	}
	return m, nil
}

// jsonError is err, from decoding data, as an *Error with a place and a reason without Go
// type names.
func jsonError(data []byte, err error) *Error {
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		line, col := position(data, syn.Offset)
		return &Error{Line: line, Col: col, Reason: syn.Error()}
	case errors.As(err, &typ):
		line, col := position(data, typ.Offset)
		what := "the file"
		if typ.Field != "" {
			what = "`" + typ.Field + "`"
		}
		return &Error{Line: line, Col: col, Reason: fmt.Sprintf("%s must be %s, not %s", what, jsonKind(typ.Type), typ.Value)}
	}
	return &Error{Line: 1, Col: 1, Reason: err.Error()}
}

// jsonKind names the JSON value a Go type decodes from.
func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Slice:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.Pointer:
		return jsonKind(t.Elem())
	case reflect.Bool:
		return "a boolean"
	}
	return "a string"
}

// position is the 1-based line and column of byte offset off in data (the decoder's offsets
// point just past the offending byte).
func position(data []byte, off int64) (int, int) {
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	if off > 0 {
		off--
	}
	line, col := 1, 1
	for _, b := range data[:off] {
		if b == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

// Load reads and parses the manifest at path in fsys: (nil, nil) when there is no such file.
func Load(fsys fs.FS, path string) (*Manifest, error) {
	data, err := fs.ReadFile(fsys, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Builtin is the embedded `alder-ui` manifest (rt/al/custom-elements.json), every element
// marked Builtin, parsed once; callers must not change it. It panics if the embedded file does
// not parse: a test keeps it parsing.
var Builtin = sync.OnceValue(func() *Manifest {
	m, err := Load(rt.Files, builtinPath)
	if err != nil || m == nil {
		panic(fmt.Sprintf("elements: the embedded manifest rt/%s does not load: %v", builtinPath, err))
	}
	for _, el := range m.Elements {
		el.Builtin = true
	}
	return m
})

// Merge is the union of builtin and app (either may be nil); for a tag both declare, app's
// declaration wins whole (its attributes and slots, and it is not Builtin).
func Merge(builtin, app *Manifest) *Manifest {
	out := &Manifest{Elements: map[string]*Element{}}
	for _, m := range []*Manifest{builtin, app} {
		if m == nil {
			continue
		}
		for tag, el := range m.Elements {
			out.Elements[tag] = el
		}
	}
	return out
}

// Escaped reports whether attribute name may be written on any custom element without its
// manifest declaring it: the global attributes `id`, `class`, `style`, `slot`, `hidden`,
// `title`, `lang`, `dir`, `tabindex`, `role`, `data-*` and `aria-*`, and Alder's own `click`,
// `submit` and `href`. (A wrapped input's `field`, `label` and `value` are the checker's: see
// WrappedInputs.)
func Escaped(name string) bool {
	switch name {
	case "id", "class", "style", "slot", "hidden", "title", "lang", "dir", "tabindex", "role",
		"click", "submit", "href":
		return true
	}
	return strings.HasPrefix(name, "data-") || strings.HasPrefix(name, "aria-")
}

// WrappedInputs maps the built-in system's input elements to the form component each one is:
// the checker treats `field`, `label` and `value` on them exactly as on that component, and
// the renderer writes the element around the component's control and its errors list.
// "Checkbox" is `<Input>` restricted to a `Bool` field.
var WrappedInputs = map[string]string{
	"al-input":    "Input",
	"al-textarea": "Textarea",
	"al-select":   "Select",
	"al-checkbox": "Checkbox",
}
