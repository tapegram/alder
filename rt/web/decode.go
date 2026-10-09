// Package web is the Alder web runtime. This file decodes form and URL-parameter fields
// (url.Values) into typed values; the generated decoders call these helpers, collect
// FieldErrors, and the runtime re-renders the page when any are reported. Imports: stdlib
// and alder/rt only.
package web

import (
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"alder/rt"
)

// FieldError is one decoding or refinement failure. Message is user-facing English without
// the field name; the generated code prefixes the field when it renders.
type FieldError struct{ Field, Message string }

// Messages for the decoders and the stdlib brands, exported so generated code and the book
// quote the same text.
const (
	MsgRequired = "is required"
	MsgInt      = "must be a whole number"
	MsgFloat    = "must be a number"
	MsgDate     = "must be a date (YYYY-MM-DD)"
	MsgNonEmpty = "must not be empty"
	MsgEmail    = "must be an email address"
	MsgURL      = "must be a URL"
	MsgSecret   = "must not be empty"
)

// Text returns the raw first value of name, untrimmed (refinements decide what is valid).
// present is true when the key exists; when required is also true, an empty value counts as
// not present. Absent keys and nil v give ("", false).
func Text(v url.Values, name string, required bool) (string, bool) {
	vs, ok := v[name]
	if !ok || len(vs) == 0 {
		return "", false
	}
	if required && vs[0] == "" {
		return "", false
	}
	return vs[0], true
}

// Int parses name as a base-10 int64 after trimming spaces. Absent or blank gives (0, ""):
// callers check presence (Text) first for required fields. Anything else unparsable or out
// of range gives (0, MsgInt).
func Int(v url.Values, name string) (int64, string) {
	s := strings.TrimSpace(v.Get(name))
	if s == "" {
		return 0, ""
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, MsgInt
	}
	return n, ""
}

// Float parses name with ParseFloat after trimming spaces. Absent or blank gives (0, "").
// Unparsable, out-of-range, NaN and infinite values give (0, MsgFloat).
func Float(v url.Values, name string) (float64, string) {
	s := strings.TrimSpace(v.Get(name))
	if s == "" {
		return 0, ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, MsgFloat
	}
	return f, ""
}

// Bool decodes a checkbox: true when the key is present and its value is neither "" nor
// "false" (case-insensitive); so "on", "true", "True", "1" are true and absent is false.
func Bool(v url.Values, name string) bool {
	s, ok := v[name]
	if !ok || len(s) == 0 {
		return false
	}
	return s[0] != "" && !strings.EqualFold(s[0], "false")
}

// MsgBool is BoolParam's error: a URL parameter of type Bool that is not one of its spellings.
const MsgBool = "must be True or False"

// BoolParam decodes a Bool URL parameter, which the app's own links write (True/False): absent
// is false; "true", "True", "1" are true and "false", "False", "0" false; anything else
// (including "") gives (false, MsgBool), which makes the route a 404. A checkbox in a form is
// decoded by Bool, leniently.
func BoolParam(v url.Values, name string) (bool, string) {
	s, ok := v[name]
	if !ok || len(s) == 0 {
		return false, ""
	}
	switch s[0] {
	case "true", "True", "1":
		return true, ""
	case "false", "False", "0":
		return false, ""
	}
	return false, MsgBool
}

// Date parses name as YYYY-MM-DD (surrounding spaces trimmed). Absent or blank gives
// (Date{}, ""); malformed or impossible dates ("2026-02-30", "2026-1-5") give MsgDate.
func Date(v url.Values, name string) (rt.Date, string) {
	s := strings.TrimSpace(v.Get(name))
	if s == "" {
		return rt.Date{}, ""
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || len(s) != 10 {
		return rt.Date{}, MsgDate
	}
	return rt.DateOf(t), ""
}

// Enum matches name case-sensitively against the constructor names and returns the index.
// No match (including absent or empty) gives (-1, "must be one of a, b, c").
func Enum(v url.Values, name string, names []string) (int, string) {
	s := v.Get(name)
	for i, n := range names {
		if s == n {
			return i, ""
		}
	}
	return -1, "must be one of " + strings.Join(names, ", ")
}

// Missing is the error for an absent required field: "is required".
func Missing(name string) FieldError { return FieldError{name, MsgRequired} }

// Check is the refinement helper: nil when ok, else a FieldError with msg.
func Check(ok bool, field, msg string) *FieldError {
	if ok {
		return nil
	}
	return &FieldError{field, msg}
}

// NonEmpty is the Text.NonEmpty brand: strings.TrimSpace(s) != "". Message MsgNonEmpty.
func NonEmpty(s string) bool { return strings.TrimSpace(s) != "" }

// Email is the Text.Email brand: exactly one "@", non-empty text on both sides, no spaces
// (any Unicode whitespace). Message MsgEmail.
func Email(s string) bool {
	if strings.Count(s, "@") != 1 {
		return false
	}
	for _, r := range s {
		if r == ' ' || strings.TrimSpace(string(r)) == "" {
			return false
		}
	}
	i := strings.IndexByte(s, '@')
	return i > 0 && i < len(s)-1
}

// URL is the Text.Url brand: url.Parse succeeds and the result has a scheme and a host
// ("mailto:a@b" and "//x" fail). Message MsgURL.
func URL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// Secret is the Text.Secret brand: s != "" (whitespace allowed). Message MsgSecret.
func Secret(s string) bool { return s != "" }
