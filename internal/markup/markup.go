// Package markup is the built-in manifest of the markup language a view writes: its tags and
// attributes, with a one-line description of each. The checker derives its tag and boolean
// attribute sets from it, and the language server reads it for hover and completion.
package markup

// Attr is one attribute.
type Attr struct {
	Name, Description string
	Boolean           bool // written bare or `={Bool}`; rendered present or absent
}

// Tag is one element and the attributes only it takes.
type Tag struct {
	Name, Description string
	Attrs             []Attr // per-tag attributes
}

// Manifest lists the tags of one markup language.
type Manifest struct {
	Name   string // "html"
	Tags   []Tag  // sorted by name
	Global []Attr // attributes every tag takes
}

// Tag is the tag called name, nil when there is none.
func (m *Manifest) Tag(name string) *Tag {
	for i := range m.Tags {
		if m.Tags[i].Name == name {
			return &m.Tags[i]
		}
	}
	return nil
}

// AttrsOf is the tag's own attributes, then the global ones.
func (m *Manifest) AttrsOf(tag string) []Attr {
	var out []Attr
	if t := m.Tag(tag); t != nil {
		out = append(out, t.Attrs...)
	}
	return append(out, m.Global...)
}

// Attr is the attribute name of tag (its own, else a global one), nil when there is none.
func (m *Manifest) Attr(tag, name string) *Attr {
	if t := m.Tag(tag); t != nil {
		for i := range t.Attrs {
			if t.Attrs[i].Name == name {
				return &t.Attrs[i]
			}
		}
	}
	for i := range m.Global {
		if m.Global[i].Name == name {
			return &m.Global[i]
		}
	}
	return nil
}
