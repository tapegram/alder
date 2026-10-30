package applog

import (
	"net/url"
	"strings"
	"testing"
)

// A file name with a short extension is readable, unless the part before the extension is
// itself a token.
func TestLooksSecret_FileExtensions(t *testing.T) {
	hex40 := "0123456789abcdef0123456789abcdef01234567"
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"Quarterly_Report_2026_Final_Version_v2.pdf", false},
		{"Team-Photo_2026-10-29_Offsite_Group.png", false},
		{"Export_Users_2026Q3_AllRegions_Final.csv", false},
		{"Archive_Backup_Database_2026_10_29.tar.gz", false},
		{hex40 + ".png", true},
		{"JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP.pdf", true}, // base32
		{jwt + ".txt", true},
		{"Ab3_xY9-Qr7Lm2Np8Kt4Vw6Zs1Ue5Hg0Jc3F", true},         // no extension: still a token
		{"Ab3_xY9-Qr7Lm2Np8Kt4Vw6Zs1Ue5Hg0Jc3F.toolong", true}, // 7 after the dot: not an extension
	} {
		if got := LooksSecret(c.s); got != c.want {
			t.Errorf("LooksSecret(%q) = %v, want %v", c.s, got, c.want)
		}
	}
	u, _ := url.ParseRequestURI("/files/Quarterly_Report_2026_Final_Version_v2.pdf")
	if got := MaskURL(u); got != "/files/Quarterly_Report_2026_Final_Version_v2.pdf" {
		t.Errorf("MaskURL = %q", got)
	}
}

// A URL nested past maxDepth is masked whole, never passed through raw.
func TestMaskURL_NestedPastDepth(t *testing.T) {
	inner3 := "https://c.example/z?q=1"
	inner2 := "https://b.example/y?c=" + url.QueryEscape(inner3)
	inner1 := "https://a.example/x?b=" + url.QueryEscape(inner2)
	u, err := url.ParseRequestURI("/start?next=" + url.QueryEscape(inner1) + "&seed=7")
	if err != nil {
		t.Fatal(err)
	}
	got := MaskURL(u)
	if strings.Contains(got, "c.example") || !strings.Contains(got, "a.example") || !strings.Contains(got, "b.example") || !strings.Contains(got, Mask) || !strings.HasSuffix(got, "&seed=7") {
		t.Errorf("MaskURL = %q", got)
	}
	// within the depth, a URL with nothing to hide stays as written
	u, _ = url.ParseRequestURI("/start?next=" + url.QueryEscape("https://b.example/y?c="+url.QueryEscape("/z")))
	if got := MaskURL(u); strings.Contains(got, Mask) {
		t.Errorf("MaskURL = %q", got)
	}
}

func TestCutPath(t *testing.T) {
	if got := CutPath("/a"); got != "/a" {
		t.Errorf("%q", got)
	}
	long := strings.Repeat("/abc", 2000)
	if got := CutPath(long); got != long[:MaxPath]+"…" {
		t.Errorf("cut to %d bytes", len(got))
	}
}
