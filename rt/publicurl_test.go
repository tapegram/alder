package rt

import "testing"

// ALDER_PUBLIC_URL's rule: http(s)://host[:port], trimmed of spaces and trailing slashes, with
// no path, query, fragment or user info; the scheme lowercased, the host as written.
func TestParsePublicURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://ex.com/":            "https://ex.com",
		" http://localhost:18080// ": "http://localhost:18080",
		"HTTPS://Ex.com":             "https://Ex.com",
		"http://127.0.0.1:8080":      "http://127.0.0.1:8080",
		"https://blog.example.com":   "https://blog.example.com",
	} {
		if got, err := ParsePublicURL(in); err != nil || got != want {
			t.Errorf("ParsePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"localhost:8080", "ftp://x", "https://ex.com/app", "https://ex.com/?a=1", "https://ex.com/#x", "https://u:p@ex.com", "https://", "https:///x"} {
		got, err := ParsePublicURL(in)
		if err == nil {
			t.Errorf("ParsePublicURL(%q) = %q, want an error", in, got)
			continue
		}
		want := `ALDER_PUBLIC_URL must be http(s)://host[:port], such as https://blog.example.com (got "` + in + `")`
		if err.Error() != want {
			t.Errorf("ParsePublicURL(%q): %q, want %q", in, err, want)
		}
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
