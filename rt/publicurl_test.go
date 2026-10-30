package rt

import (
	"strconv"
	"testing"
)

// ALDER_PUBLIC_URL's rule: http(s)://host[:port], trimmed of spaces and trailing slashes, with
// no path, query, fragment or user info; the scheme lowercased, the host as written.
func TestParsePublicURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://ex.com/":            "https://ex.com",
		" http://localhost:18080// ": "http://localhost:18080",
		"HTTPS://Ex.com":             "https://Ex.com",
		"http://127.0.0.1:8080":      "http://127.0.0.1:8080",
		"https://blog.example.com":   "https://blog.example.com",
		"http://[::1]:9000":          "http://[::1]:9000",
		"https://ex.com:65535":       "https://ex.com:65535",
	} {
		if got, err := ParsePublicURL(in); err != nil || got != want {
			t.Errorf("ParsePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"localhost:8080", "ftp://x", "https://ex.com/app", "https://ex.com/?a=1", "https://ex.com/#x", "https://u:p@ex.com", "https://", "https:///x",
		"https://ex.com:", "https://ex.com:99999", "https://ex.com:0", "https://ex.com:+80", "https://ex.com#", "https://ex.com?", "https://ex.com;x", `https://ex.com"><script>`,
		"https://ex_com", "https://[1.2.3.4]", "https://ex.com\\evil"} {
		got, err := ParsePublicURL(in)
		if err == nil {
			t.Errorf("ParsePublicURL(%q) = %q, want an error", in, got)
			continue
		}
		want := `ALDER_PUBLIC_URL must be http(s)://host[:port], such as https://blog.example.com (got ` + strconv.Quote(in) + `)`
		if err.Error() != want {
			t.Errorf("ParsePublicURL(%q): %q, want %q", in, err, want)
		}
	}
}

// The served executable's base, from its environment before it listens: ALDER_PUBLIC_URL, else
// the address it will listen on; a port 0 is refined after Listen.
func TestResolvePublicURL(t *testing.T) {
	for _, c := range []struct {
		env    map[string]string
		want   string
		refine bool
	}{
		{map[string]string{}, "http://localhost:8080", false},
		{map[string]string{"ALDER_ADDR": "localhost:9000"}, "http://localhost:9000", false},
		{map[string]string{"ALDER_ADDR": "127.0.0.1:0"}, "http://127.0.0.1:0", true},
		{map[string]string{"ALDER_ADDR": ":0", "ALDER_PUBLIC_URL": " "}, "http://localhost:0", true},
		{map[string]string{"ALDER_ADDR": ":0", "ALDER_PUBLIC_URL": "https://ex.com/"}, "https://ex.com", false},
	} {
		got, refine, err := ResolvePublicURL(func(k string) string { return c.env[k] })
		if err != nil || got != c.want || refine != c.refine {
			t.Errorf("%v: %q %v %v; want %q %v", c.env, got, refine, err, c.want, c.refine)
		}
	}
	if _, _, err := ResolvePublicURL(func(k string) string { return map[string]string{"ALDER_PUBLIC_URL": "ex.com"}[k] }); err == nil {
		t.Error("a bad ALDER_PUBLIC_URL resolved")
	}
}

// Without ALDER_PUBLIC_URL the base is the listener's address as alder run shows it: an
// unspecified host is localhost.
func TestListenPublicURL(t *testing.T) {
	for in, want := range map[string]string{
		"[::]:8080":       "http://localhost:8080",
		"0.0.0.0:8080":    "http://localhost:8080",
		":8080":           "http://localhost:8080",
		"127.0.0.1:18765": "http://127.0.0.1:18765",
		"[::1]:9000":      "http://[::1]:9000",
	} {
		if got := ListenPublicURL(in); got != want {
			t.Errorf("ListenPublicURL(%q) = %q, want %q", in, got, want)
		}
	}
	if PublicURL != DefaultPublicURL || DefaultPublicURL != "http://localhost:8080" {
		t.Errorf("PublicURL %q, DefaultPublicURL %q", PublicURL, DefaultPublicURL)
	}
}
