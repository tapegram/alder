package applog

import (
	"net/url"
	"strings"
	"testing"
)

// Every key word Sensitive names, in each case and embedded in a longer name: a miss leaks a
// secret, so each one is its own row.
func TestSensitive(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"key", true}, {"KEY", true}, {"api_key", true}, {"X-Api-Key", true}, {"keyring", true},
		{"token", true}, {"Token", true}, {"access_token", true}, {"refresh_token", true}, {"csrfToken", true},
		{"secret", true}, {"SECRET", true}, {"client_secret", true}, {"secretName", true},
		{"pass", true}, {"password", true}, {"Passwd", true}, {"passphrase", true}, {"user_pass", true},
		{"auth", true}, {"Authorization", true}, {"oauth_verifier", true}, {"AUTH", true},
		{"sig", true}, {"signature", true}, {"X-Amz-Signature", true}, {"sig_v4", true},
		{"code", true}, {"CODE", true}, {"auth_code", true}, {"zipcode", true},
		{"session", true}, {"SessionId", true}, {"alder_session", true},
		{"", false}, {"seed", false}, {"page", false}, {"q", false}, {"id", false}, {"sort", false},
		{"name", false}, {"email", false}, {"datastar", false}, {"limit", false},
	} {
		if got := Sensitive(c.name); got != c.want {
			t.Errorf("Sensitive(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// The checker's cases (internal/check: E0030's corpus) and the edges of each kind.
func TestLooksSecret(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"postgres://user:pa55@db.example:5432/app", true},
		{"smtp://user:smtp-pass@mail.example:25", true},
		{"https://user@host/path", false},     // a user without a password
		{"https://host/p?x=a:b@c", false},     // the @ is past the authority
		{"libsql://app-acme.turso.io", false}, // no credentials
		{"sk_live_abc", true}, {"sk_test_x", true}, {"ghp_abcdef", true}, {"AKIAIOSFODNN7EXAMPLE", true},
		{"xoxb-123", true}, {"xoxp-456", true}, {"-----BEGIN PRIVATE KEY-----", true},
		{strings.Repeat("a", 31), false},
		{strings.Repeat("a", 32), true},
		{"0123456789abcdef0123456789abcdef0123456789", true},           // 40 hex
		{"YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo0MTIzNDU2Nzg5MA==", true}, // base64
		{"abc+def/ghi=" + strings.Repeat("x", 30), true},               // base64 with / and =
		{"/usr/local/share/very/long/path/to/some/file", false},
		{"./a/very/long/relative/path/to/some/file/x", false},
		{"static/a/very/long/relative/path/to/somefile", false},
		{strings.Repeat("a", 20) + " " + strings.Repeat("b", 20), false}, // a space
		{strings.Repeat("a", 40) + "-", false},                           // a character outside the alphabet
		{"", false}, {"hello", false}, {"7", false},
	} {
		if got := LooksSecret(c.s); got != c.want {
			t.Errorf("LooksSecret(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestMaskURL(t *testing.T) {
	hex40 := "0123456789abcdef0123456789abcdef01234567"
	for _, c := range []struct{ in, want string }{
		// nightvote-replay's case, and keys and order kept
		{"/start?key=s3cret&seed=7", "/start?key=••••&seed=7"},
		{"/start?seed=7&key=s3cret", "/start?seed=7&key=••••"},
		{"/start?seed=7", "/start?seed=7"},
		{"/start", "/start"},
		{"/", "/"},
		// every key word
		{"/x?token=a", "/x?token=••••"},
		{"/x?access_token=a&state=b", "/x?access_token=••••&state=b"},
		{"/x?Authorization=Bearer+x", "/x?Authorization=••••"},
		{"/x?secret=a", "/x?secret=••••"},
		{"/x?password=hunter2", "/x?password=••••"},
		{"/x?sig=a&expires=1", "/x?sig=••••&expires=1"},
		{"/x?X-Amz-Signature=a", "/x?X-Amz-Signature=••••"},
		{"/cb?code=a&state=b", "/cb?code=••••&state=b"},
		{"/x?session=a", "/x?session=••••"},
		{"/x?API_KEY=a", "/x?API_KEY=••••"},
		{"/x?auth=a", "/x?auth=••••"},
		// an encoded key is judged by its decoded name
		{"/x?%6Bey=a", "/x?%6Bey=••••"},
		// a secret-looking value under a plain name
		{"/x?q=sk_live_abc", "/x?q=••••"},
		{"/x?q=" + hex40, "/x?q=••••"},
		{"/x?next=postgres%3A%2F%2Fu%3Ap%40h%2Fdb", "/x?next=••••"},
		{"/x?q=ghp%5Fabc", "/x?q=••••"}, // judged decoded
		// a key that itself looks like a secret
		{"/x?sk_live_abc", "/x?••••"},
		{"/x?" + hex40 + "=1", "/x?••••=1"},
		// repeated keys, empty values, empty pairs, a key without "="
		{"/x?key=a&key=b", "/x?key=••••&key=••••"},
		{"/x?key=&seed=", "/x?key=••••&seed="},
		{"/x?a=1&&b=2", "/x?a=1&&b=2"},
		{"/x?token", "/x?token"},
		{"/x?flag&key=a", "/x?flag&key=••••"},
		// unparsable: masked pair by pair, never dropped
		{"/x?q=%zz&seed=7", "/x?q=••••&seed=7"},
		{"/x?key=%zz&seed=7", "/x?key=••••&seed=7"},
		{"/x?%zz=1&seed=7", "/x?%zz=••••&seed=7"}, // the pair does not parse: its value is masked
		{"/x?%zzkey=1", "/x?%zzkey=••••"},
		{"/x?a=1;token=abc&seed=7", "/x?a=••••&seed=7"},
		{"/x?x;token=abc", "/x?x;token=••••"},
		// path segments that look like secrets
		{"/r/" + hex40 + "/start", "/r/••••/start"},
		{"/r/sk_live_abc", "/r/••••"},
		{"/r/" + hex40 + "/x?key=a&seed=7", "/r/••••/x?key=••••&seed=7"},
		{"/r/sk%5Flive%5Fabc", "/r/••••"}, // judged decoded too
		{"/r/normal-slug/42", "/r/normal-slug/42"},
		// characters that need escaping stay escaped: no raw newline reaches a line
		{"/a%0Ab", "/a%0Ab"},
	} {
		u, err := url.ParseRequestURI(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got := MaskURL(u); got != c.want {
			t.Errorf("MaskURL(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A secret anywhere in the query or the path is never in the result, whatever surrounds it.
func TestMaskURL_SecretNeverKept(t *testing.T) {
	const s = "sk_live_TOPSECRET"
	for _, raw := range []string{
		"/x?key=" + s, "/x?q=" + s, "/x?" + s, "/" + s, "/a/" + s + "/b", "/x?a=1;b=" + s,
		"/x?token=" + s + "%zz", "/x?q=%zz" + s, "/x?password=" + s + "&x=1",
	} {
		u, err := url.ParseRequestURI(raw)
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got := MaskURL(u); strings.Contains(got, "TOPSECRET") {
			t.Errorf("MaskURL(%s) = %q keeps the secret", raw, got)
		}
	}
}

func TestMaskURL_PlainPathNoAlloc(t *testing.T) {
	u, _ := url.ParseRequestURI("/posts/42/edit")
	if n := testing.AllocsPerRun(100, func() { _ = MaskURL(u) }); n != 0 {
		t.Errorf("MaskURL of a plain path: %v allocations, want 0", n)
	}
	if got := MaskURL(u); got != "/posts/42/edit" {
		t.Errorf("MaskURL = %q", got)
	}
}
