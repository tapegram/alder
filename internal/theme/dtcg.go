package theme

// The structural layer of the DTCG reader (W3C Design Tokens, format 2025.10): it reads a
// resolver document and the token files it refers to, or a single tokens file, and returns
// every token with its path, type, raw value and origin, sorted into root, light and dark in
// file order. It knows nothing of alder's token names: ImportDTCG maps the tokens onto them.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	dtcgMaxBytes = 1 << 20 // the largest document the reader accepts
	dtcgMaxFiles = 32      // the most files one resolver may refer to
	// dtcgMaxSources is the most file and set `$ref`s one document may expand, counting a
	// source each time it is reached: it bounds a resolver whose sets refer to each other many
	// times over.
	dtcgMaxSources = 1024
)

// schemeModifierNames are the names a resolver's light/dark modifier may have, matched
// case-insensitively.
var schemeModifierNames = []string{"scheme", "theme", "mode", "color-scheme", "colorScheme"}

// dtcgToken is one design token as the file wrote it.
type dtcgToken struct {
	Path        []string // e.g. ["color","primary"]; a single file's light/dark group is not part of it
	Type        string   // its own `$type`, else the nearest group's; "" if none
	Value       json.RawMessage
	Description string
	File        string // the file it came from (a path in the fs.FS), for Unplaced.Block
}

// dtcgDoc is a loaded DTCG document: a resolver with its files, or one tokens file.
type dtcgDoc struct {
	Name, Description string      // the resolver's name and description ("" for a tokens file)
	Root, Light, Dark []dtcgToken // file order; a path defined twice keeps its first place and the later value
	Skipped           Skipped     // the modifiers resolved at their default (or first) context

	// byPath indexes each list by dotted path ("color.primary"), for alias lookup: see lookup.
	byPath struct{ root, light, dark map[string]dtcgToken }
}

// lookup finds the token an alias `{path}` names, as the light (dark false) or dark scheme
// sees the document: the scheme's own tokens first, then the root's.
func (d *dtcgDoc) lookup(dotted string, dark bool) (dtcgToken, bool) {
	scheme := d.byPath.light
	if dark {
		scheme = d.byPath.dark
	}
	if t, ok := scheme[dotted]; ok {
		return t, true
	}
	t, ok := d.byPath.root[dotted]
	return t, ok
}

// loadDTCG reads name from fsys. A document with `resolutionOrder` is a resolver: its sets
// merge into Root in order (later wins), the first modifier named like a scheme whose contexts
// include light and dark supplies Light and Dark, and any other modifier is resolved at its
// default context (else its first) into Root and counted in Skipped. Any other document is a
// tokens file: its top-level `light` and `dark` groups are the schemes, the rest is Root.
// `$ref`s resolve against name's directory and must stay inside fsys. Each file is read and
// walked once however often it is referred to, and more than dtcgMaxSources expanded sources
// is an error.
//
// Root, Light and Dark are kept apart: a scheme's value for a path beats Root's wherever the
// sets that supplied Root sit in resolutionOrder, since lookup and ImportDTCG read the
// scheme before Root.
func loadDTCG(fsys fs.FS, name string) (*dtcgDoc, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("`%s` is not a path inside the theme's directory", name)
	}
	raw, err := readDTCGFile(fsys, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("`%s` is not there", name)
		}
		return nil, err
	}
	top, ok, err := jsonMembers(raw)
	if err != nil {
		return nil, fmt.Errorf("`%s`: %v", name, err)
	}
	if !ok {
		return nil, fmt.Errorf("`%s` is not a JSON object", name)
	}
	l := &dtcgLoader{fsys: fsys, name: name, dir: path.Dir(name), files: map[string]bool{}, parsed: map[string][]dtcgToken{}}
	doc := &dtcgDoc{}
	if _, isResolver := member(top, "resolutionOrder"); isResolver {
		err = l.resolver(top, doc)
	} else {
		err = l.tokensFile(top)
	}
	if err != nil {
		return nil, err
	}
	doc.Root, doc.byPath.root = l.root.toks, l.root.byPath()
	doc.Light, doc.byPath.light = l.light.toks, l.light.byPath()
	doc.Dark, doc.byPath.dark = l.dark.toks, l.dark.byPath()
	doc.Skipped = l.sk
	return doc, nil
}

type dtcgLoader struct {
	fsys              fs.FS
	name, dir         string                 // the document loadDTCG was given, and its directory
	files             map[string]bool        // the files `$ref`s have named so far
	parsed            map[string][]dtcgToken // each file's tokens, walked once
	expanded          int                    // the file and set sources expanded so far
	root, light, dark tokenList
	sk                Skipped
	scheme            bool // the scheme modifier has been found
	sets              map[string]json.RawMessage
	modifiers         map[string]json.RawMessage
	inSet             map[string]bool // the sets being expanded, against `#/sets/` cycles
}

// tokensFile places a document read alone: top-level `light`/`dark` groups (any case) are the
// schemes, without their own name in the path; everything else is root.
func (l *dtcgLoader) tokensFile(top []jsonMember) error {
	typ, err := groupType(top, "", l.name, nil)
	if err != nil {
		return err
	}
	var rest []jsonMember
	for _, m := range top {
		list := l.schemeList(m)
		if list == nil {
			rest = append(rest, m)
			continue
		}
		sub, _, _ := jsonMembers(m.val)
		if err := walkTokens(sub, nil, typ, l.name, list); err != nil {
			return err
		}
	}
	// rest still holds top's `$type`, so it is re-read as the same inherited type.
	return walkTokens(rest, nil, "", l.name, &l.root)
}

// schemeList returns the list a top-level member of a tokens file fills when it is a `light`
// or `dark` group, else nil.
func (l *dtcgLoader) schemeList(m jsonMember) *tokenList {
	var list *tokenList
	switch {
	case strings.EqualFold(m.key, "light"):
		list = &l.light
	case strings.EqualFold(m.key, "dark"):
		list = &l.dark
	default:
		return nil
	}
	sub, ok, err := jsonMembers(m.val)
	if err != nil || !ok {
		return nil
	}
	if _, isToken := member(sub, "$value"); isToken {
		return nil
	}
	return list
}

func (l *dtcgLoader) resolver(top []jsonMember, doc *dtcgDoc) error {
	for _, m := range top {
		var err error
		switch m.key {
		case "name":
			err = json.Unmarshal(m.val, &doc.Name)
		case "description":
			err = json.Unmarshal(m.val, &doc.Description)
		case "sets":
			l.sets, err = l.namedObjects(m.val)
		case "modifiers":
			l.modifiers, err = l.namedObjects(m.val)
		}
		if err != nil {
			return fmt.Errorf("`%s`: `%s` is not valid: %v", l.name, m.key, err)
		}
	}
	order, _ := member(top, "resolutionOrder")
	var items []json.RawMessage
	if err := json.Unmarshal(order, &items); err != nil {
		return fmt.Errorf("`%s`: `resolutionOrder` is not a list", l.name)
	}
	for _, item := range items {
		if err := l.orderItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (l *dtcgLoader) namedObjects(raw json.RawMessage) (map[string]json.RawMessage, error) {
	ms, ok, err := jsonMembers(raw)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("not an object")
	}
	out := make(map[string]json.RawMessage, len(ms))
	for _, m := range ms {
		out[m.key] = m.val
	}
	return out, nil
}

// orderItem applies one entry of `resolutionOrder`: a `$ref` to a set or modifier, or an
// inline one with `type` and `name`.
func (l *dtcgLoader) orderItem(item json.RawMessage) error {
	ms, ok, err := jsonMembers(item)
	if err != nil || !ok {
		return fmt.Errorf("`%s`: a `resolutionOrder` entry is not an object", l.name)
	}
	if ref, has, err := stringMember(ms, "$ref"); err != nil {
		return fmt.Errorf("`%s`: %v", l.name, err)
	} else if has {
		if set, ok := strings.CutPrefix(ref, "#/sets/"); ok {
			return l.set(pointerUnescape(set), &l.root)
		}
		if mod, ok := strings.CutPrefix(ref, "#/modifiers/"); ok {
			raw, ok := l.modifiers[pointerUnescape(mod)]
			if !ok {
				return fmt.Errorf("`%s`: `resolutionOrder` names `%s`, which is not a modifier", l.name, ref)
			}
			return l.modifier(pointerUnescape(mod), raw)
		}
		return fmt.Errorf("`%s`: `resolutionOrder` entry `%s` is not a set or a modifier", l.name, ref)
	}
	kind, _, err := stringMember(ms, "type")
	if err != nil {
		return fmt.Errorf("`%s`: %v", l.name, err)
	}
	name, _, err := stringMember(ms, "name")
	if err != nil {
		return fmt.Errorf("`%s`: %v", l.name, err)
	}
	switch kind {
	case "set":
		sources, _ := member(ms, "sources")
		return l.sources(sources, "set `"+name+"`", &l.root)
	case "modifier":
		return l.modifier(name, item)
	}
	return fmt.Errorf("`%s`: a `resolutionOrder` entry has neither `$ref` nor a `type` of set or modifier", l.name)
}

// set merges the sources of the set named name into list, guarding against a set that
// includes itself through `#/sets/` sources.
func (l *dtcgLoader) set(name string, list *tokenList) error {
	raw, ok := l.sets[name]
	if !ok {
		return fmt.Errorf("`%s`: `#/sets/%s` is not a set", l.name, name)
	}
	if l.inSet[name] {
		return fmt.Errorf("`%s`: set `%s` includes itself", l.name, name)
	}
	if err := l.expand(); err != nil {
		return err
	}
	ms, ok, err := jsonMembers(raw)
	if err != nil || !ok {
		return fmt.Errorf("`%s`: set `%s` is not an object", l.name, name)
	}
	if l.inSet == nil {
		l.inSet = map[string]bool{}
	}
	l.inSet[name] = true
	defer delete(l.inSet, name)
	sources, _ := member(ms, "sources")
	return l.sources(sources, "set `"+name+"`", list)
}

// modifier places a modifier's contexts: the scheme's light and dark, or any other at its
// default (else first) context into Root, counted in Skipped.
func (l *dtcgLoader) modifier(name string, raw json.RawMessage) error {
	ms, ok, err := jsonMembers(raw)
	if err != nil || !ok {
		return fmt.Errorf("`%s`: modifier `%s` is not an object", l.name, name)
	}
	ctxRaw, _ := member(ms, "contexts")
	contexts, ok, err := jsonMembers(ctxRaw)
	if err != nil || !ok || len(contexts) == 0 {
		return fmt.Errorf("`%s`: modifier `%s` has no contexts", l.name, name)
	}
	what := func(ctx string) string { return "modifier `" + name + "` context `" + ctx + "`" }

	if !l.scheme && isSchemeModifierName(name) {
		var light, dark *jsonMember
		for i := range contexts {
			switch {
			case light == nil && strings.EqualFold(contexts[i].key, "light"):
				light = &contexts[i]
			case dark == nil && strings.EqualFold(contexts[i].key, "dark"):
				dark = &contexts[i]
			}
		}
		if light != nil && dark != nil {
			l.scheme = true
			if err := l.sources(light.val, what(light.key), &l.light); err != nil {
				return err
			}
			return l.sources(dark.val, what(dark.key), &l.dark)
		}
	}

	chosen := contexts[0]
	if def, has, err := stringMember(ms, "default"); err != nil {
		return fmt.Errorf("`%s`: modifier `%s`: %v", l.name, name, err)
	} else if has {
		raw, ok := member(contexts, def)
		if !ok {
			return fmt.Errorf("`%s`: modifier `%s` defaults to `%s`, which is not one of its contexts", l.name, name, def)
		}
		chosen = jsonMember{def, raw}
	}
	l.sk.N++
	if !slices.Contains(l.sk.Kinds, name) {
		l.sk.Kinds = append(l.sk.Kinds, name)
	}
	return l.sources(chosen.val, what(chosen.key), &l.root)
}

func isSchemeModifierName(name string) bool {
	for _, s := range schemeModifierNames {
		if strings.EqualFold(name, s) {
			return true
		}
	}
	return false
}

// sources reads a list of sources into list: each a `$ref` to a file or to a set, or an
// inline group of tokens.
func (l *dtcgLoader) sources(raw json.RawMessage, what string, list *tokenList) error {
	var srcs []json.RawMessage
	if err := json.Unmarshal(raw, &srcs); err != nil {
		return fmt.Errorf("`%s`: %s: the sources are not a list", l.name, what)
	}
	for _, src := range srcs {
		ms, ok, err := jsonMembers(src)
		if err != nil || !ok {
			return fmt.Errorf("`%s`: %s: a source is not an object", l.name, what)
		}
		ref, has, err := stringMember(ms, "$ref")
		if err != nil {
			return fmt.Errorf("`%s`: %s: %v", l.name, what, err)
		}
		if !has {
			if err := walkTokens(ms, nil, "", l.name, list); err != nil {
				return err
			}
			continue
		}
		if set, ok := strings.CutPrefix(ref, "#/sets/"); ok {
			if err := l.set(pointerUnescape(set), list); err != nil {
				return err
			}
			continue
		}
		if err := l.file(ref, list); err != nil {
			return err
		}
	}
	return nil
}

// file reads the tokens file a source's `$ref` names, relative to the resolver.
func (l *dtcgLoader) file(ref string, list *tokenList) error {
	if strings.Contains(ref, "#") {
		return fmt.Errorf("`%s`: `$ref` `%s`: only whole files and `#/sets/…` can be referred to", l.name, ref)
	}
	if !fs.ValidPath(ref) {
		return fmt.Errorf("`%s`: `$ref` `%s` must be a relative path inside the theme's directory", l.name, ref)
	}
	p := path.Join(l.dir, ref)
	if !l.files[p] {
		l.files[p] = true
		if len(l.files) > dtcgMaxFiles {
			return fmt.Errorf("`%s` refers to more than %d files", l.name, dtcgMaxFiles)
		}
	}
	if err := l.expand(); err != nil {
		return err
	}
	toks, ok := l.parsed[p]
	if !ok {
		var err error
		if toks, err = l.readTokens(p); err != nil {
			return err
		}
		l.parsed[p] = toks
	}
	for _, t := range toks {
		list.add(t)
	}
	return nil
}

// expand counts one more expanded source against dtcgMaxSources.
func (l *dtcgLoader) expand() error {
	l.expanded++
	if l.expanded > dtcgMaxSources {
		return fmt.Errorf("`%s` expands more than %d sources", l.name, dtcgMaxSources)
	}
	return nil
}

// readTokens reads and walks the tokens file at p, in file order.
func (l *dtcgLoader) readTokens(p string) ([]dtcgToken, error) {
	raw, err := readDTCGFile(l.fsys, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("`%s` refers to `%s`, which is not there", l.name, p)
		}
		return nil, err
	}
	ms, ok, err := jsonMembers(raw)
	if err != nil {
		return nil, fmt.Errorf("`%s`: %v", p, err)
	}
	if !ok {
		return nil, fmt.Errorf("`%s` is not a JSON object", p)
	}
	var list tokenList
	if err := walkTokens(ms, nil, "", p, &list); err != nil {
		return nil, err
	}
	return list.toks, nil
}

// readDTCGFile reads one JSON document of at most dtcgMaxBytes, without a leading BOM.
func readDTCGFile(fsys fs.FS, name string) (json.RawMessage, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, dtcgMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("`%s`: %v", name, err)
	}
	if len(b) > dtcgMaxBytes {
		return nil, fmt.Errorf("`%s` is over 1 MiB", name)
	}
	b = bytes.TrimPrefix(b, []byte("\uFEFF"))
	dec := json.NewDecoder(bytes.NewReader(b))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("`%s` is not valid JSON: %v", name, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("`%s` is not valid JSON: more after the document", name)
	}
	return raw, nil
}

// walkTokens adds the tokens of a group's members to list, in order. typ is the `$type` the
// group inherits; path is the group's own.
func walkTokens(ms []jsonMember, path []string, typ, file string, list *tokenList) error {
	typ, err := groupType(ms, typ, file, path)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if strings.HasPrefix(m.key, "$") && m.key != "$root" {
			continue // $type, $description, $extensions, $deprecated, …
		}
		p := append(append([]string(nil), path...), m.key)
		sub, ok, err := jsonMembers(m.val)
		if err != nil || !ok {
			return fmt.Errorf("`%s`: `%s` is neither a token nor a group", file, strings.Join(p, "."))
		}
		value, isToken := member(sub, "$value")
		if !isToken {
			if err := walkTokens(sub, p, typ, file, list); err != nil {
				return err
			}
			continue
		}
		own, err := groupType(sub, typ, file, p)
		if err != nil {
			return err
		}
		t := dtcgToken{Path: p, Type: own, Value: value, File: file}
		if d, ok := member(sub, "$description"); ok {
			_ = json.Unmarshal(d, &t.Description) // a description that is not a string is dropped
		}
		list.add(t)
	}
	return nil
}

// groupType returns the members' own `$type`, else inherited.
func groupType(ms []jsonMember, inherited, file string, path []string) (string, error) {
	typ, has, err := stringMember(ms, "$type")
	if err != nil {
		where := strings.Join(path, ".")
		if where == "" {
			where = "the top level"
		}
		return "", fmt.Errorf("`%s`: %s: %v", file, where, err)
	}
	if !has {
		return inherited, nil
	}
	return typ, nil
}

// tokenList holds tokens in first-seen order; adding a path again replaces the value in place.
type tokenList struct {
	toks []dtcgToken
	at   map[string]int
}

func (l *tokenList) add(t dtcgToken) {
	k := strings.Join(t.Path, ".")
	if i, ok := l.at[k]; ok {
		l.toks[i] = t
		return
	}
	if l.at == nil {
		l.at = map[string]int{}
	}
	l.at[k] = len(l.toks)
	l.toks = append(l.toks, t)
}

func (l *tokenList) byPath() map[string]dtcgToken {
	m := make(map[string]dtcgToken, len(l.toks))
	for k, i := range l.at {
		m[k] = l.toks[i]
	}
	return m
}

// jsonMember is one key of a JSON object, with its value undecoded.
type jsonMember struct {
	key string
	val json.RawMessage
}

// jsonMembers lists an object's members in source order; ok is false when raw is not an
// object.
func jsonMembers(raw json.RawMessage) (ms []jsonMember, ok bool, err error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, false, err
	}
	if d, isDelim := tok.(json.Delim); !isDelim || d != '{' {
		return nil, false, nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false, err
		}
		ms = append(ms, jsonMember{tok.(string), v})
	}
	return ms, true, nil
}

// member returns the last value of key (JSON's usual reading of a repeated key).
func member(ms []jsonMember, key string) (json.RawMessage, bool) {
	for i := len(ms) - 1; i >= 0; i-- {
		if ms[i].key == key {
			return ms[i].val, true
		}
	}
	return nil, false
}

func stringMember(ms []jsonMember, key string) (string, bool, error) {
	raw, ok := member(ms, key)
	if !ok {
		return "", false, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", true, fmt.Errorf("`%s` is not a string", key)
	}
	return s, true, nil
}

// pointerUnescape undoes JSON Pointer escaping in one reference token.
func pointerUnescape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
}
