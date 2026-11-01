package theme

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Equivalent reports whether a and b are the same value of a token of kind k, by meaning
// rather than spelling. Colours match when both parse and every channel, alpha included, is
// within 1/255; lengths match by number and unit, any zero matching any other (`0` ≡ `0px`),
// with no conversion between units (`0.5rem` ≢ `8px`); shadows match layer by layer, each
// layer's lengths and colour compared as above and `inset` anywhere in it; fonts match as
// family lists, quotes optional. Anything the rules cannot read (`var(--primary)`) matches
// only the same text, whitespace collapsed.
func Equivalent(k Kind, a, b string) bool {
	switch k {
	case Colour:
		return colourEquivalent(a, b)
	case Length:
		return lengthEquivalent(a, b)
	case Shadow:
		return shadowEquivalent(a, b)
	case Font:
		return slices.Equal(Families(a), Families(b))
	}
	return collapse(a) == collapse(b)
}

func colourEquivalent(a, b string) bool {
	ca, okA := ParseColour(a)
	cb, okB := ParseColour(b)
	if !okA || !okB {
		return collapse(a) == collapse(b)
	}
	const tol = 1.0/255 + 1e-9
	return math.Abs(ca.R-cb.R) <= tol && math.Abs(ca.G-cb.G) <= tol &&
		math.Abs(ca.B-cb.B) <= tol && math.Abs(ca.A-cb.A) <= tol
}

func lengthEquivalent(a, b string) bool {
	na, ua, okA := splitLength(a)
	nb, ub, okB := splitLength(b)
	if !okA || !okB {
		return collapse(a) == collapse(b)
	}
	if na == 0 && nb == 0 {
		return true
	}
	return na == nb && ua == ub
}

// splitLength reads a length, negative allowed, as its number and lower-cased unit.
func splitLength(v string) (float64, string, bool) {
	v = strings.TrimSpace(v)
	if !LengthOK(v) && !NegativeLength(v) {
		return 0, "", false
	}
	i := strings.IndexFunc(v, func(r rune) bool { return r != '-' && r != '.' && (r < '0' || r > '9') })
	if i < 0 {
		i = len(v)
	}
	n, err := strconv.ParseFloat(strings.ReplaceAll(v[:i], " ", ""), 64)
	if err != nil {
		return 0, "", false
	}
	return n, strings.ToLower(v[i:]), true
}

func shadowEquivalent(a, b string) bool {
	la, lb := splitTop(a, ','), splitTop(b, ',')
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if !layerEquivalent(la[i], lb[i]) {
			return false
		}
	}
	return true
}

// shadowLayer is one layer of a box-shadow: whether it is inset, its lengths in order, and
// the rest (its colour, or a word the rules cannot read) in order.
type shadowLayer struct {
	inset   bool
	lengths []string
	rest    []string
}

func parseLayer(s string) shadowLayer {
	var l shadowLayer
	for _, part := range splitTop(collapse(s), ' ') {
		switch {
		case part == "":
		case strings.EqualFold(part, "inset"):
			l.inset = true
		case LengthOK(part) || NegativeLength(part):
			l.lengths = append(l.lengths, part)
		default:
			l.rest = append(l.rest, part)
		}
	}
	return l
}

func layerEquivalent(a, b string) bool {
	la, lb := parseLayer(a), parseLayer(b)
	if la.inset != lb.inset || len(la.lengths) != len(lb.lengths) || len(la.rest) != len(lb.rest) {
		return false
	}
	for i := range la.lengths {
		if !lengthEquivalent(la.lengths[i], lb.lengths[i]) {
			return false
		}
	}
	for i := range la.rest {
		if !colourEquivalent(la.rest[i], lb.rest[i]) {
			return false
		}
	}
	return true
}
