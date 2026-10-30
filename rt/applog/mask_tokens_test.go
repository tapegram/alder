package applog

import (
	"net/url"
	"strings"
	"testing"
)

// JWTs, base64url tokens, URL-valued parameters, raw bytes and the extra key words are masked;
// UUIDs, slugs, file names and short ids stay readable.
func TestMaskURL_Tokens(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	b64url := "Ab3_xY9-Qr7Lm2Np8Kt4Vw6Zs1Ue5Hg0Jc3Fd7Bn9Mq2Pk4Rt6Yw8Ex"
	if len(b64url) != 55 {
		t.Fatalf("b64url is %d characters", len(b64url))
	}
	uuid := "550e8400-e29b-41d4-a716-446655440000"
	for _, c := range []struct{ in, want string }{
		{"/reset/" + jwt, "/reset/••••"},
		{"/x?t=" + jwt, "/x?t=••••"},
		{"/x?" + jwt, "/x?••••"},
		{"/r/" + b64url, "/r/••••"},
		{"/x?q=" + b64url, "/x?q=••••"},
		{"/x?" + b64url + "=1", "/x?••••=1"},
		{"/x?jwt=abc&pw=x&pwd=y&otp=123456&bearer=z&credential=c&Credentials=d", "/x?jwt=••••&pw=••••&pwd=••••&otp=••••&bearer=••••&credential=••••&Credentials=••••"},
		// a URL-valued parameter: its inner URL masked by the same rules
		{"/login?redirect=https%3A%2F%2Fhost%2Fcb%3Fapi_key%3Dzzz%26page%3D2", "/login?redirect=https%3A%2F%2Fhost%2Fcb%3Fapi_key%3D••••%26page%3D2"},
		{"/login?next=%2Fcb%3Ftoken%3Dabc", "/login?next=%2Fcb%3Ftoken%3D••••"},
		{"/login?next=%2Fr%2F" + b64url, "/login?next=%2Fr%2F••••"},
		{"/login?next=https%3A%2F%2Fhost%2Fa%3Fpage%3D2", "/login?next=https%3A%2F%2Fhost%2Fa%3Fpage%3D2"}, // nothing to hide: as written
		{"/login?next=/plain/path", "/login?next=/plain/path"},
		{"/cb?u=https%3A%2F%2Fh%2F%23access_token%3Dabc", "/cb?u=https%3A%2F%2Fh%2F%23access_token%3D••••"}, // a fragment's pairs too
		// UUIDs, slugs, file names and short ids are not secrets
		{"/u/" + uuid + "/items/12345?id=" + uuid, "/u/" + uuid + "/items/12345?id=" + uuid},
		{"/x?id=" + strings.ToUpper(uuid), "/x?id=" + strings.ToUpper(uuid)},
		{"/posts/how-to-write-a-long-blog-post-slug-2026", "/posts/how-to-write-a-long-blog-post-slug-2026"},
		{"/files/report.final.version.2026.october.pdf", "/files/report.final.version.2026.october.pdf"},
		{"/x?id=abc123&page=7", "/x?id=abc123&page=7"},
		// raw bytes >= 0x80 (a C1 control among them) are %-escaped, never written raw
		{"/x?q=\u009b2J", "/x?q=%C2%9B2J"},
		{"/x?caf\xc3\xa9=1", "/x?caf%C3%A9=1"},
	} {
		u, err := url.ParseRequestURI(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got := MaskURL(u); got != c.want {
			t.Errorf("MaskURL(%s)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestLooksSecret_Tokens(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc", true},
		{"eyJ.no", false},
		{"Ab3_xY9-Qr7Lm2Np8Kt4Vw6Zs1Ue5Hg0Jc3F", true}, // base64url, mixed case and digits
		{"550e8400-e29b-41d4-a716-446655440000", false},
		{"550E8400-E29B-41D4-A716-446655440000", false},
		{"how-to-write-a-long-blog-post-slug-2026", false},
		{"report.final.version.2026.october.pdf", false},
	} {
		if got := LooksSecret(c.s); got != c.want {
			t.Errorf("LooksSecret(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestSensitive_MoreWords(t *testing.T) {
	for _, n := range []string{"pw", "new_pw", "pwd", "PWD", "jwt", "id_jwt", "otp", "OTP_code", "bearer", "Bearer", "credential", "credentials", "AWS_CREDENTIAL"} {
		if !Sensitive(n) {
			t.Errorf("Sensitive(%q) = false", n)
		}
	}
}
