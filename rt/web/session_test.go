package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// sessionPayload is the signed payload the router stores for the session JSON s, issued at iss
// (unix seconds; it expires sessionMaxAge later).
func sessionPayload(s string, iss int64) []byte {
	return []byte(fmt.Sprintf(`{"iss":%d,"session":%s}`, iss, s))
}

func TestSession_TamperedCookieIsDefault(t *testing.T) {
	key := []byte("0123456789abcdef0123")
	c := Sign(key, sessionPayload(`{"User":"ada"}`, time.Now().Unix()-60))
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
	for _, cookie := range []string{string(b), "garbage", Sign(key, sessionPayload("not json", time.Now().Unix()-60))} {
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
			o.Dev = true
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
		if len(logs) != 1 || logs[0] != "warn: "+devWarning {
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

// Outside dev mode a missing or short secret is refused (C2): CheckSecret reports it, and
// NewRouter panics rather than sign sessions with the public development key.
func TestSession_ProdNeedsSecret(t *testing.T) {
	for _, secret := range [][]byte{nil, []byte("short"), []byte("fifteen-bytes!!")} {
		if err := CheckSecret(secret, false); err != ErrNoSecret {
			t.Fatalf("CheckSecret(%q, prod) = %v", secret, err)
		}
		if err := CheckSecret(secret, true); err != nil {
			t.Fatalf("CheckSecret(%q, dev) = %v", secret, err)
		}
		func() {
			defer func() {
				if v := recover(); v == nil || !strings.Contains(fmt.Sprint(v), ErrNoSecret.Error()) {
					t.Fatalf("NewRouter with secret %q outside dev: panic %v", secret, v)
				}
			}()
			NewRouter(nil, testOpts(func(o *Options) { o.Secret = secret }))
		}()
	}
	if err := CheckSecret([]byte("sixteen-bytes!!!"), false); err != nil {
		t.Fatal(err)
	}
	if ErrNoSecret.Error() != "ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)" {
		t.Fatal(ErrNoSecret)
	}
}

// The signed payload carries the time the session was issued (I6, final-review I1): the cookie
// expires 30 days after it, however often it is sent — the router stores the cookie only when a
// response changes the session, so a visit does not renew it, and a cookie past its 30 days (or
// without an issued time) is no session, whatever the browser keeps.
func TestSession_Expiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	var seen any
	routes := []Route{
		{Method: "GET", Pattern: "/login", Handle: func(rq *Request) Response {
			return PageResponse(doc("in"), nil, testSession{User: "ada"}, false)
		}},
		{Method: "GET", Pattern: "/who", Handle: func(rq *Request) Response {
			seen = rq.Session
			return PageResponse(doc("who"), nil, rq.Session, false) // a page answers with the session it saw
		}},
		{Method: "GET", Pattern: "/rename", Handle: func(rq *Request) Response {
			return PageResponse(doc("renamed"), nil, testSession{User: "bob"}, false)
		}},
		{Method: "GET", Pattern: "/relogin", Handle: func(rq *Request) Response {
			rq.Env.SessionPut = true // a Session.put that wrote the value the session already had
			return PageResponse(doc("again"), nil, rq.Session, false)
		}},
	}
	key := []byte("test-secret-test-secret")
	h := NewRouter(routes, testOpts(func(o *Options) { o.Now = func() time.Time { return now } }))
	get := func(path string, c *http.Cookie) (any, *http.Cookie) {
		req := httptest.NewRequest("GET", path, nil)
		if c != nil {
			req.AddCookie(c)
		}
		seen = nil
		w := serve(h, req)
		return seen, sessionCookie(t, w)
	}
	// a login (the session changes) issues a 30-day cookie
	_, c := get("/login", nil)
	if c == nil || c.MaxAge != sessionMaxAge {
		t.Fatalf("login cookie %+v", c)
	}
	if p, ok := Verify(key, c.Value); !ok || string(p) != string(sessionPayload(`{"User":"ada"}`, 1_000_000)) {
		t.Fatalf("payload %s", p)
	}
	ada := testSession{User: "ada"}
	// an unchanged session is not stored: no Set-Cookie, the cookie keeps its issued time
	now = now.Add(29*24*time.Hour + 23*time.Hour + 59*time.Minute) // 60 s left
	s, again := get("/who", c)
	if s != ada || again != nil {
		t.Fatalf("with 60 s left: %#v, Set-Cookie %+v", s, again)
	}
	// so, however often it is sent, it expires 30 days after it was issued: a visitor
	now = now.Add(time.Minute)
	if s, again := get("/who", c); s != (testSession{}) || again != nil {
		t.Fatalf("30 days after the login: %#v, Set-Cookie %+v", s, again)
	}
	// a changed session is stored, issued now
	now = time.Unix(2_000_000, 0)
	_, c = get("/login", nil)
	now = now.Add(time.Hour)
	_, renamed := get("/rename", c)
	if p, ok := Verify(key, renamed.Value); !ok || string(p) != string(sessionPayload(`{"User":"bob"}`, 2_003_600)) {
		t.Fatalf("changed session payload %s", p)
	}
	// a Session.put is a change even when it writes the same value: logging in again as the
	// user already signed in is issued anew
	now = now.Add(time.Hour)
	_, re := get("/relogin", renamed)
	if p, ok := Verify(key, re.Value); !ok || string(p) != string(sessionPayload(`{"User":"bob"}`, 2_007_200)) {
		t.Fatalf("Session.put of the same session: payload %s", p)
	}
	// a visitor's default session is not stored either
	if _, v := get("/who", nil); v != nil {
		t.Fatalf("visitor Set-Cookie %+v", v)
	}
	// exactly 30 days after it was issued: expired
	iss := now.Unix() - sessionMaxAge
	for _, payload := range [][]byte{
		sessionPayload(`{"User":"ada"}`, iss),
		[]byte(`{"session":{"User":"ada"}}`),                  // no issued time
		[]byte(`{"User":"ada"}`),                              // the cookie of a build before expiries
		[]byte(`{"exp":9999999999,"session":{"User":"ada"}}`), // the cookie of a build before issued times
		[]byte(`{"iss":"soon","session":{"User":"ada"}}`),
	} {
		if s, _ := get("/who", &http.Cookie{Name: CookieName, Value: Sign(key, payload)}); s != (testSession{}) {
			t.Fatalf("payload %s: %#v", payload, s)
		}
	}
	if s, _ := get("/who", &http.Cookie{Name: CookieName, Value: Sign(key, sessionPayload(`{"User":"ada"}`, iss+1))}); s != ada {
		t.Fatalf("one second before the expiry: %#v", s)
	}
}
