package theme

// The value layer of the DTCG reader: cssText turns a token's DTCG value into the CSS text a
// declaration holds, knowing nothing of alder's names; ImportDTCG maps each token onto
// alder-ui's table and checks its value as the CSS importer does.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// dtcgValueError is a value the reader cannot turn into CSS text: ImportDTCG unplaces the token
// with it as the reason. Any other error from cssText (an alias cycle) fails the import.
type dtcgValueError string

func (e dtcgValueError) Error() string { return string(e) }

// dtcgTypeKinds are the `$type`s alder-ui has tokens of, and the kind each holds.
var dtcgTypeKinds = map[string]Kind{"color": Colour, "dimension": Length, "fontFamily": Font, "shadow": Shadow}

// colourAlias writes an alias to a colour-typed token: its text and true, or false to have the
// target's value substituted.
type colourAlias func(target dtcgToken) (string, bool)

// cssText is the CSS text of t's value as the scheme ("light", "dark", or "" for the root)
// sees the document. An alias to a colour-typed token is `var(--<its last segment>)`; an
// alias to anything else is the target's text.
func (d *dtcgDoc) cssText(t dtcgToken, scheme string) (string, error) {
	return d.text(t, scheme, func(target dtcgToken) (string, bool) {
		return "var(--" + dtcgSegment(target.Path) + ")", true
	})
}

func (d *dtcgDoc) text(t dtcgToken, scheme string, colourVar colourAlias) (string, error) {
	c := &dtcgConv{d: d, scheme: scheme, colourVar: colourVar, seen: []string{strings.Join(t.Path, ".")}}
	typ, err := c.typeOf(t)
	if err != nil {
		return "", err
	}
	return c.value(t.Value, typ)
}

// dtcgConv converts one token's value; seen is the alias chain that led to the value being
// converted, against cycles.
type dtcgConv struct {
	d         *dtcgDoc
	scheme    string
	colourVar colourAlias
	seen      []string
}

// resolve finds the token an alias names: the scheme's first, then the root's; for the root
// (scheme ""), the root's first, then either scheme's.
func (c *dtcgConv) resolve(ref string) (dtcgToken, bool) {
	switch c.scheme {
	case "light":
		return c.d.lookup(ref, false)
	case "dark":
		return c.d.lookup(ref, true)
	}
	for _, m := range []map[string]dtcgToken{c.d.byPath.root, c.d.byPath.light, c.d.byPath.dark} {
		if t, ok := m[ref]; ok {
			return t, true
		}
	}
	return dtcgToken{}, false
}

// typeOf is t's `$type`, or for an untyped alias the type of what it names; "" when neither.
func (c *dtcgConv) typeOf(t dtcgToken) (string, error) {
	seen := slices.Clone(c.seen)
	for t.Type == "" {
		ref, ok := dtcgAlias(t.Value)
		if !ok {
			return "", nil
		}
		if err := c.cycle(seen, ref); err != nil {
			return "", err
		}
		seen = append(seen, ref)
		if t, ok = c.resolve(ref); !ok {
			return "", nil
		}
	}
	return t.Type, nil
}

func (c *dtcgConv) cycle(seen []string, ref string) error {
	if !slices.Contains(seen, ref) {
		return nil
	}
	return fmt.Errorf("alias cycle: `{%s}` → `{%s}`", strings.Join(seen, "}` → `{"), ref)
}

// value converts raw as a value of type typ.
func (c *dtcgConv) value(raw json.RawMessage, typ string) (string, error) {
	if ref, ok := dtcgAlias(raw); ok {
		return c.alias(ref, typ)
	}
	switch typ {
	case "color":
		return colourText(raw)
	case "dimension":
		return dimensionText(raw)
	case "fontFamily":
		return fontText(raw)
	case "shadow":
		return c.shadow(raw)
	case "":
		return "", dtcgValueError("no $type")
	}
	return "", dtcgValueError("alder-ui has no `" + typ + "` tokens")
}

// alias converts `{ref}`: a colour-typed target as colourVar writes it, anything else (or a
// colour colourVar declines) by substituting the target's value.
func (c *dtcgConv) alias(ref, typ string) (string, error) {
	if err := c.cycle(c.seen, ref); err != nil {
		return "", err
	}
	target, ok := c.resolve(ref)
	if !ok {
		return "", dtcgValueError("`{" + ref + "}` is not a token")
	}
	c.seen = append(c.seen, ref)
	defer func() { c.seen = c.seen[:len(c.seen)-1] }()
	ttyp, err := c.typeOf(target)
	if err != nil {
		return "", err
	}
	if ttyp == "" {
		ttyp = typ
	}
	if ttyp == "color" {
		if err := c.chain(target); err != nil {
			return "", err
		}
		if v, ok := c.colourVar(target); ok {
			return v, nil
		}
	}
	return c.value(target.Value, ttyp)
}

// chain follows the aliases from t, which a var() would leave to the browser, for a cycle.
func (c *dtcgConv) chain(t dtcgToken) error {
	seen := slices.Clone(c.seen)
	for {
		ref, ok := dtcgAlias(t.Value)
		if !ok {
			return nil
		}
		if err := c.cycle(seen, ref); err != nil {
			return err
		}
		seen = append(seen, ref)
		if t, ok = c.resolve(ref); !ok {
			return nil
		}
	}
}

// dtcgAlias is the path a `"{a.b}"` value names.
func dtcgAlias(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) != nil || len(s) < 3 || s[0] != '{' || s[len(s)-1] != '}' {
		return "", false
	}
	ref := s[1 : len(s)-1]
	return ref, !strings.ContainsAny(ref, "{}")
}

// colourFunctions are the colour spaces written as their own CSS function; any other space
// alder-ui can carry is written `color(<space> …)`.
var colourFunctions = map[string]bool{"oklch": true, "oklab": true, "lab": true, "lch": true, "hsl": true, "hwb": true}

var colourSpaces = map[string]bool{
	"srgb-linear": true, "display-p3": true, "a98-rgb": true, "prophoto-rgb": true, "rec2020": true,
	"xyz-d65": true, "xyz-d50": true, "xyz": true,
}

func colourText(raw json.RawMessage) (string, error) {
	var v struct {
		ColorSpace string            `json:"colorSpace"`
		Components []json.RawMessage `json:"components"`
		Alpha      *float64          `json:"alpha"`
		Hex        string            `json:"hex"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.ColorSpace == "" {
		return "", dtcgValueError("not a DTCG colour: expected an object with `colorSpace` and `components`")
	}
	if len(v.Components) != 3 {
		return "", dtcgValueError("a colour has three `components`")
	}
	space := v.ColorSpace
	opaque := v.Alpha == nil || *v.Alpha == 1
	if space == "srgb" && v.Hex != "" && opaque {
		return v.Hex, nil
	}
	parts := make([]string, 3)
	for i, raw := range v.Components {
		var s string
		if json.Unmarshal(raw, &s) == nil && s == "none" {
			parts[i] = "none"
			continue
		}
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return "", dtcgValueError("a colour component is a number or `\"none\"`")
		}
		switch {
		case space == "srgb":
			parts[i] = cssNumber(math.Round(f*255*1000) / 1000)
		case (space == "hsl" || space == "hwb") && i > 0:
			parts[i] = cssNumber(f) + "%"
		default:
			parts[i] = cssNumber(f)
		}
	}
	body := strings.Join(parts, " ")
	if !opaque {
		body += " / " + cssNumber(*v.Alpha)
	}
	switch {
	case space == "srgb":
		return "rgb(" + body + ")", nil
	case colourFunctions[space]:
		return space + "(" + body + ")", nil
	case colourSpaces[space]:
		return "color(" + space + " " + body + ")", nil
	}
	return "", dtcgValueError("colour space `" + space + "` is not one alder-ui can write")
}

var reUnit = regexp.MustCompile(`^[a-z%]+$`)

func dimensionText(raw json.RawMessage) (string, error) {
	var v struct {
		Value *float64 `json:"value"`
		Unit  string   `json:"unit"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.Value == nil || !reUnit.MatchString(v.Unit) {
		return "", dtcgValueError("not a DTCG dimension: expected an object with `value` and `unit`")
	}
	if *v.Value == 0 {
		return "0", nil
	}
	return cssNumber(*v.Value) + v.Unit, nil
}

// fontText is a family or a list of them, joined as a Theme holds a font stack.
func fontText(raw json.RawMessage) (string, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one, nil
	}
	var fams []string
	if err := json.Unmarshal(raw, &fams); err != nil || len(fams) == 0 {
		return "", dtcgValueError("not a DTCG font family: expected a name or a list of names")
	}
	return strings.Join(fams, ", "), nil
}

// shadow is one layer or a list of them, joined by `, `.
func (c *dtcgConv) shadow(raw json.RawMessage) (string, error) {
	var layers []json.RawMessage
	if json.Unmarshal(raw, &layers) != nil {
		layers = []json.RawMessage{raw}
	}
	if len(layers) == 0 {
		return "", dtcgValueError("a shadow has at least one layer")
	}
	out := make([]string, len(layers))
	for i, layer := range layers {
		var err error
		if _, ok := dtcgAlias(layer); ok {
			out[i], err = c.value(layer, "shadow")
		} else {
			out[i], err = c.shadowLayer(layer)
		}
		if err != nil {
			return "", err
		}
	}
	return strings.Join(out, ", "), nil
}

func (c *dtcgConv) shadowLayer(raw json.RawMessage) (string, error) {
	var v struct {
		Color, OffsetX, OffsetY, Blur, Spread json.RawMessage
		Inset                                 bool
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.Color == nil || v.OffsetX == nil || v.OffsetY == nil || v.Blur == nil || v.Spread == nil {
		return "", dtcgValueError("not a DTCG shadow: each layer has `color`, `offsetX`, `offsetY`, `blur` and `spread`")
	}
	var parts []string
	if v.Inset {
		parts = append(parts, "inset")
	}
	for _, d := range []json.RawMessage{v.OffsetX, v.OffsetY, v.Blur, v.Spread} {
		s, err := c.value(d, "dimension")
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	col, err := c.value(v.Color, "color")
	if err != nil {
		return "", err
	}
	return strings.Join(append(parts, col), " "), nil
}

// cssNumber prints f in Go's shortest form: `0.3`, never `.3`.
func cssNumber(f float64) string {
	if f == 0 {
		return "0" // and not `-0`
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// dtcgSegment is the name a token goes by: its last path segment (its group's for a `$root`
// token), lower-cased with `_`, camelCase humps and anything a CSS name cannot hold as `-`.
func dtcgSegment(path []string) string {
	seg := path[len(path)-1]
	if seg == "$root" && len(path) > 1 {
		seg = path[len(path)-2]
	}
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		switch ch := seg[i]; {
		case 'A' <= ch && ch <= 'Z':
			if i > 0 && ('a' <= seg[i-1] && seg[i-1] <= 'z' || '0' <= seg[i-1] && seg[i-1] <= '9') {
				b.WriteByte('-')
			}
			b.WriteByte(ch + 'a' - 'A')
		case 'a' <= ch && ch <= 'z', '0' <= ch && ch <= '9', ch == '-':
			b.WriteByte(ch)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ---- ImportDTCG

// ImportDTCG reads a DTCG resolver or tokens file (see loadDTCG) into a Theme, the tokens it
// could not place and the modifiers it resolved at their default. A colour in Root goes to
// both schemes, and a scheme's own value beats it. A theme-wide token in Light goes to Root;
// one in Dark is placed only when Root's value is equivalent (Equivalent), and unplaced as differing or, with no Root value, as set only by the dark scheme. Each token's name is looked up in alder-ui's
// table; its value is converted (cssText, an alias to an alder-ui colour kept as its var())
// and checked as the CSS importer checks it. It is an error when no token was placed.
func ImportDTCG(fsys fs.FS, name string) (Theme, []Unplaced, Skipped, error) {
	d, err := loadDTCG(fsys, name)
	if err != nil {
		return Theme{}, nil, Skipped{}, err
	}
	im := &dtcgImporter{d: d, th: Theme{Root: map[string]string{}, Light: map[string]string{}, Dark: map[string]string{}}}
	for _, list := range []struct {
		toks   []dtcgToken
		scheme string
	}{{d.Root, ""}, {d.Light, "light"}, {d.Dark, "dark"}} {
		for _, t := range list.toks {
			if err := im.place(t, list.scheme); err != nil {
				return Theme{}, nil, Skipped{}, fmt.Errorf("`%s`: %v", t.File, err)
			}
		}
	}
	for _, p := range im.pending {
		v, ok := im.th.Root[p.field]
		tok, _ := Lookup(p.field)
		switch {
		case ok && Equivalent(tok.Kind, v, p.val):
		case !ok:
			p.u.Reason = "theme-wide in alder-ui; only the dark scheme sets it"
			im.un = append(im.un, p.u)
		default:
			im.un = append(im.un, p.u)
		}
	}
	if im.th.Empty() {
		return im.th, im.un, d.Skipped, fmt.Errorf("no token of alder-ui %s in `%s`", Version(), name)
	}
	return im.th, im.un, d.Skipped, nil
}

type dtcgImporter struct {
	d       *dtcgDoc
	th      Theme
	un      []Unplaced
	pending []pendingDark // theme-wide tokens of the dark scheme, placed when Root agrees
}

// dtcgName is the alder-ui token a normalised segment names.
func dtcgName(seg string) (Token, bool) {
	if t, ok := ByCSS("--" + seg); ok {
		return t, true
	}
	if t, ok := Lookup(seg); ok {
		return t, true
	}
	return ByCSS("--al-" + seg)
}

// text converts as cssText does, but keeps an alias's var() only for a colour of alder-ui.
func (im *dtcgImporter) text(t dtcgToken, scheme string) (string, error) {
	return im.d.text(t, scheme, func(target dtcgToken) (string, bool) {
		if tok, ok := dtcgName(dtcgSegment(target.Path)); ok && tok.Kind == Colour {
			return "var(" + tok.CSS + ")", true
		}
		return "", false
	})
}

func (im *dtcgImporter) place(t dtcgToken, scheme string) error {
	seg := dtcgSegment(t.Path)
	tok, known := dtcgName(seg)
	u := Unplaced{Block: t.File + " " + strings.Join(t.Path, "."), Name: "--" + seg}
	if scheme == "dark" {
		u.Scheme = "dark"
	}
	if known {
		u.Name = tok.CSS
	}
	unplace := func(value, reason string) {
		u.Value, u.Reason = value, reason
		im.un = append(im.un, u)
	}
	// convert is the token's text for a scheme; ok is false when it was unplaced instead.
	convert := func(scheme string) (string, bool, error) {
		s, err := im.text(t, scheme)
		var bad dtcgValueError
		if errors.As(err, &bad) {
			unplace(dtcgComment(t.Value), string(bad))
			return "", false, nil
		}
		return s, err == nil, err
	}

	if _, ok := dtcgTypeKinds[t.Type]; t.Type != "" && !ok {
		unplace(dtcgComment(t.Value), "alder-ui has no `"+t.Type+"` tokens")
		return nil
	}
	if !known {
		s, ok, err := convert(scheme)
		if !ok {
			return err
		}
		base := strings.TrimPrefix(seg, "al-")
		if why, removed := Removed[FieldName("--al-al-"+base)]; removed {
			unplace(s, "`--al-"+base+"` was removed in alder-ui v0.4: "+why)
		} else {
			unplace(s, "not a token of alder-ui "+Version())
		}
		return nil
	}
	typ, err := (&dtcgConv{d: im.d, scheme: scheme, seen: []string{strings.Join(t.Path, ".")}}).typeOf(t)
	if err != nil {
		return err
	}
	if kind, ok := dtcgTypeKinds[typ]; ok && kind != tok.Kind {
		unplace(dtcgComment(t.Value), fmt.Sprintf("`%s` is a %s; the file says `%s`", tok.Field, tok.Kind, typ))
		return nil
	}

	schemes := []string{scheme}
	if tok.Kind == Colour && scheme == "" {
		schemes = []string{"light", "dark"}
	}
	vals := make([]string, len(schemes))
	for i, sch := range schemes {
		s, ok, err := convert(sch)
		if !ok {
			return err
		}
		if tok.Kind == Length && NegativeLength(s) {
			unplace(s, NegativeLengthReason(tok.Field))
			return nil
		}
		val, reason := check(tok, s)
		if reason != "" {
			unplace(s, reason)
			return nil
		}
		vals[i] = val
	}
	for i, sch := range schemes {
		switch {
		case tok.Kind == Colour && sch == "light":
			im.th.Light[tok.Field] = vals[i]
		case tok.Kind == Colour:
			im.th.Dark[tok.Field] = vals[i]
		case sch == "dark":
			u.Value, u.Reason = vals[i], "theme-wide in alder-ui; the dark value differs"
			im.pending = append(im.pending, pendingDark{field: tok.Field, val: vals[i], u: u})
		default:
			im.th.Root[tok.Field] = vals[i]
		}
	}
	return nil
}

// dtcgComment is a value as the file wrote it, as a CSS comment: the Unplaced.Value of a token
// that produced no CSS text.
func dtcgComment(raw json.RawMessage) string {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		b.Reset()
		b.Write(raw)
	}
	return "/* " + strings.ReplaceAll(b.String(), "*/", "* /") + " */"
}
