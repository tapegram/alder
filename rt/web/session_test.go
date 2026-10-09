package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSession_RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123")
	for _, p := range [][]byte{[]byte(`{"user":"ada"}`), {}, []byte("a.b.c")} {
		got, ok := Verify(key, Sign(key, p))
		if !ok || !bytes.Equal(got, p) {
			t.Fatalf("Verify(Sign(%q)) = %q, %v", p, got, ok)
		}
	}
	for _, bad := range []string{"", ".", "abc", "!!!.!!!", "a.b.c"} {
		if _, ok := Verify(key, bad); ok {
			t.Fatalf("Verify(%q) accepted", bad)
		}
	}
}

func TestSession_TamperedCookieIsDefault(t *testing.T) {
	key := []byte("0123456789abcdef0123")
	c := Sign(key, []byte(`{"User":"ada"}`))
	// Flip one byte of the payload part.
	b := []byte(c)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	if _, ok := Verify(key, string(b)); ok {
		t.Fatal("tampered cookie verified")
	}

	var seen []any
	routes := []Route{{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
		seen = append(seen, rq.Session, rq.Env.Session)
		return PageResponse(doc("home"), nil, nil, false)
	}}}
	h := NewRouter(routes, testOpts(func(o *Options) { o.Secret = key }))
	for _, cookie := range []string{string(b), "garbage", Sign(key, []byte("not json"))} {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
	}
	// And a good cookie decodes.
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: c})
	h.ServeHTTP(httptest.NewRecorder(), req)
	want := []any{testSession{}, testSession{}, testSession{}, testSession{}, testSession{}, testSession{}, testSession{User: "ada"}, testSession{User: "ada"}}
	if len(seen) != len(want) {
		t.Fatalf("seen %v", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen[%d] = %#v, want %#v", i, seen[i], want[i])
		}
	}
}

func TestSession_WrongKeyRejected(t *testing.T) {
	c := Sign([]byte("key-one-key-one-key-one"), []byte(`{"User":"ada"}`))
	if _, ok := Verify([]byte("key-two-key-two-key-two"), c); ok {
		t.Fatal("cookie signed with another key verified")
	}
	if _, ok := Verify(DevKey(), c); ok {
		t.Fatal("cookie verified with the dev key")
	}
}

func TestSession_DevKeyWarnsOnce(t *testing.T) {
	for _, secret := range [][]byte{nil, []byte("short")} {
		var logs []string
		opts := testOpts(func(o *Options) {
			o.Secret = secret
			o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
		})
		var got any
		routes := []Route{
			{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
				return PageResponse(doc("home"), nil, testSession{User: "ada"}, false)
			}},
			{Method: "GET", Pattern: "/who", Handle: func(rq *Request) Response {
				got = rq.Session
				return PageResponse(doc("who"), nil, nil, false)
			}},
		}
		h := NewRouter(routes, opts)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		req := httptest.NewRequest("GET", "/who", nil)
		for _, c := range w.Result().Cookies() {
			req.AddCookie(c)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if len(logs) != 1 || logs[0] != "warn: warning: ALDER_SECRET not set; using the development cookie key" {
			t.Fatalf("secret %q: logs %q", secret, logs)
		}
		// The cookie was signed with the dev key.
		if got != (testSession{User: "ada"}) {
			t.Fatalf("session through the dev key = %#v", got)
		}
		c := w.Result().Cookies()[0]
		if _, ok := Verify(DevKey(), c.Value); !ok {
			t.Fatal("cookie not signed with DevKey")
		}
	}
	// A real secret: no warning.
	var logs []string
	h := NewRouter(nil, testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs = append(logs, msg) }
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	for _, l := range logs {
		if strings.Contains(l, "ALDER_SECRET") {
			t.Fatalf("warned with a secret: %q", logs)
		}
	}
}

func TestSession_BigCookieWarnsOnce(t *testing.T) {
	var logs []string
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(doc("x"), nil, testSession{User: strings.Repeat("u", 4000)}, false)
	}), testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
	}))
	for i := 0; i < 3; i++ {
		if c := sessionCookie(t, serve(h, httptest.NewRequest("GET", "/", nil))); c == nil {
			t.Fatal("no cookie")
		}
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "warn: the session cookie is ") {
		t.Fatalf("logs %q", logs)
	}
	// Secure behind a TLS-terminating proxy.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if c := sessionCookie(t, serve(h, req)); c == nil || !c.Secure {
		t.Fatalf("cookie %+v", c)
	}
}
