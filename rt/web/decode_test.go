package web

import (
	"net/url"
	"testing"

	"alder/rt"
)

func vals(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Add(kv[i], kv[i+1])
	}
	return v
}

func TestText(t *testing.T) {
	cases := []struct {
		name     string
		v        url.Values
		required bool
		want     string
		present  bool
	}{
		{"valid", vals("f", "hi"), true, "hi", true},
		{"whitespace kept", vals("f", "  hi "), true, "  hi ", true},
		{"unicode", vals("f", "héllo ✓"), true, "héllo ✓", true},
		{"absent required", vals(), true, "", false},
		{"absent optional", vals(), false, "", false},
		{"empty required", vals("f", ""), true, "", false},
		{"empty optional", vals("f", ""), false, "", true},
		{"nil values", nil, true, "", false},
	}
	for _, c := range cases {
		got, ok := Text(c.v, "f", c.required)
		if got != c.want || ok != c.present {
			t.Errorf("%s: got (%q,%v) want (%q,%v)", c.name, got, ok, c.want, c.present)
		}
	}
}

func TestInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		msg  string
	}{
		{"42", 42, ""}, {"-7", -7, ""}, {" 5 ", 5, ""}, {"", 0, ""}, {"  ", 0, ""},
		{"1.5", 0, MsgInt}, {"abc", 0, MsgInt}, {"٣", 0, MsgInt}, {"99999999999999999999", 0, MsgInt},
		{"9223372036854775807", 9223372036854775807, ""},
	}
	for _, c := range cases {
		got, msg := Int(vals("f", c.in), "f")
		if got != c.want || msg != c.msg {
			t.Errorf("%q: got (%d,%q) want (%d,%q)", c.in, got, msg, c.want, c.msg)
		}
	}
	if got, msg := Int(vals(), "f"); got != 0 || msg != "" {
		t.Errorf("absent: got (%d,%q)", got, msg)
	}
}

func TestFloat(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		msg  string
	}{
		{"1.5", 1.5, ""}, {"-2", -2, ""}, {" 3.25 ", 3.25, ""}, {"1e3", 1000, ""}, {"", 0, ""},
		{"abc", 0, MsgFloat}, {"1,5", 0, MsgFloat}, {"NaN", 0, MsgFloat}, {"Inf", 0, MsgFloat}, {"1e999", 0, MsgFloat},
	}
	for _, c := range cases {
		got, msg := Float(vals("f", c.in), "f")
		if got != c.want || msg != c.msg {
			t.Errorf("%q: got (%v,%q) want (%v,%q)", c.in, got, msg, c.want, c.msg)
		}
	}
}

func TestBool(t *testing.T) {
	cases := []struct {
		name string
		v    url.Values
		want bool
	}{
		{"on", vals("f", "on"), true}, {"true", vals("f", "true"), true}, {"True", vals("f", "True"), true},
		{"1", vals("f", "1"), true}, {"false", vals("f", "false"), false}, {"FALSE", vals("f", "FALSE"), false},
		{"empty", vals("f", ""), false}, {"absent", vals(), false}, {"unicode", vals("f", "✓"), true},
	}
	for _, c := range cases {
		if got := Bool(c.v, "f"); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestDate(t *testing.T) {
	cases := []struct {
		in   string
		want rt.Date
		msg  string
	}{
		{"2026-01-05", rt.Date{Y: 2026, M: 1, D: 5}, ""},
		{" 2026-01-05 ", rt.Date{Y: 2026, M: 1, D: 5}, ""},
		{"2024-02-29", rt.Date{Y: 2024, M: 2, D: 29}, ""},
		{"", rt.Date{}, ""},
		{"2026-02-30", rt.Date{}, MsgDate},
		{"2025-02-29", rt.Date{}, MsgDate},
		{"2026-1-5", rt.Date{}, MsgDate},
		{"2026/01/05", rt.Date{}, MsgDate},
		{"2026-13-01", rt.Date{}, MsgDate},
		{"abc", rt.Date{}, MsgDate},
		{"２０２６-01-05", rt.Date{}, MsgDate},
	}
	for _, c := range cases {
		got, msg := Date(vals("f", c.in), "f")
		if got != c.want || msg != c.msg {
			t.Errorf("%q: got (%v,%q) want (%v,%q)", c.in, got, msg, c.want, c.msg)
		}
	}
}

func TestEnum(t *testing.T) {
	names := []string{"Red", "Green", "Blue"}
	cases := []struct {
		in   string
		want int
		msg  string
	}{
		{"Red", 0, ""}, {"Blue", 2, ""}, {"red", -1, "must be one of Red, Green, Blue"},
		{"", -1, "must be one of Red, Green, Blue"}, {" Red", -1, "must be one of Red, Green, Blue"},
		{"Pink", -1, "must be one of Red, Green, Blue"},
	}
	for _, c := range cases {
		got, msg := Enum(vals("f", c.in), "f", names)
		if got != c.want || msg != c.msg {
			t.Errorf("%q: got (%d,%q) want (%d,%q)", c.in, got, msg, c.want, c.msg)
		}
	}
	if got, msg := Enum(vals(), "f", names); got != -1 || msg == "" {
		t.Errorf("absent: got (%d,%q)", got, msg)
	}
	if got, msg := Enum(vals("f", "x"), "f", nil); got != -1 || msg != "must be one of " {
		t.Errorf("no names: got (%d,%q)", got, msg)
	}
}

func TestMissingAndCheck(t *testing.T) {
	if e := Missing("name"); e != (FieldError{"name", "is required"}) {
		t.Errorf("Missing: %v", e)
	}
	if e := Check(true, "f", "m"); e != nil {
		t.Errorf("Check ok: %v", e)
	}
	if e := Check(false, "f", "m"); e == nil || *e != (FieldError{"f", "m"}) {
		t.Errorf("Check fail: %v", e)
	}
}

func TestPredicates(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) bool
		in   string
		want bool
	}{
		{"NonEmpty", NonEmpty, "a", true}, {"NonEmpty", NonEmpty, " ", false}, {"NonEmpty", NonEmpty, "", false},
		{"NonEmpty", NonEmpty, "\t\n", false}, {"NonEmpty", NonEmpty, " é ", true},
		{"Email", Email, "a@b", true}, {"Email", Email, "@b", false}, {"Email", Email, "a@", false},
		{"Email", Email, "a @b", false}, {"Email", Email, "a@b@c", false}, {"Email", Email, "ab", false},
		{"Email", Email, "", false}, {"Email", Email, "é@ü.de", true}, {"Email", Email, "a@b ", false},
		{"URL", URL, "http://x", true}, {"URL", URL, "https://a.b/c?d=1", true}, {"URL", URL, "x", false},
		{"URL", URL, "mailto:a@b", false}, {"URL", URL, "", false}, {"URL", URL, "http://", false},
		{"URL", URL, "//x", false}, {"URL", URL, "http://[::1", false}, {"URL", URL, " ", false},
		{"Secret", Secret, "x", true}, {"Secret", Secret, " ", true}, {"Secret", Secret, "", false},
	}
	for _, c := range cases {
		if got := c.fn(c.in); got != c.want {
			t.Errorf("%s(%q) = %v want %v", c.name, c.in, got, c.want)
		}
	}
	if MsgNonEmpty != "must not be empty" || MsgEmail != "must be an email address" ||
		MsgURL != "must be a URL" || MsgSecret != "must not be empty" {
		t.Error("message constants changed")
	}
}

// A Bool URL parameter is strict (M3): the app's links write True/False; anything else is an
// error (the route answers 404). A checkbox stays lenient (Bool).
func TestBoolParam(t *testing.T) {
	for raw, want := range map[string]bool{"true": true, "True": true, "1": true, "false": false, "False": false, "0": false} {
		if got, msg := BoolParam(url.Values{"on": {raw}}, "on"); got != want || msg != "" {
			t.Errorf("BoolParam(%q) = %v, %q", raw, got, msg)
		}
	}
	for _, raw := range []string{"maybe", "", "on", "TRUE", "yes"} {
		if got, msg := BoolParam(url.Values{"on": {raw}}, "on"); got || msg != MsgBool {
			t.Errorf("BoolParam(%q) = %v, %q", raw, got, msg)
		}
	}
	if got, msg := BoolParam(url.Values{}, "on"); got || msg != "" {
		t.Errorf("absent: %v, %q", got, msg)
	}
	if !Bool(url.Values{"on": {"on"}}, "on") {
		t.Error("a checkbox's on is true")
	}
}
