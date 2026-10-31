package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"alder/internal/elements"
	"alder/internal/theme"
	"alder/rt/datastar"
)

// The gallery shows every alder-ui element of the vendored manifest but the dev panel and the
// toast region, and no `al-*` tag the manifest lacks (spec "Layout").
func TestGallery_MatchesManifest(t *testing.T) {
	m := elements.Builtin()
	excluded := map[string]bool{"al-devpanel": true, "al-toasts": true}
	var tags []string
	for tag := range m.Elements {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		if excluded[tag] {
			if regexp.MustCompile(`<` + tag + `[\s>/]`).MatchString(themeGallery) {
				t.Errorf("the gallery shows %s, which it leaves out", tag)
			}
			continue
		}
		if !regexp.MustCompile(`<` + tag + `[\s>/]`).MatchString(themeGallery) {
			t.Errorf("the gallery has no <%s>", tag)
		}
	}
	for _, tag := range regexp.MustCompile(`<(al-[a-z-]+)`).FindAllStringSubmatch(themeGallery, -1) {
		if m.Elements[tag[1]] == nil {
			t.Errorf("the gallery has <%s>, which the manifest does not declare", tag[1])
		}
	}
}

// The panel's own stylesheet uses no theme token, so a working theme can never hide the panel
// (spec "Layout").
func TestPanelCSS_NoThemeTokens(t *testing.T) {
	tokens := map[string]bool{}
	for _, tk := range theme.Tokens {
		tokens[tk.CSS] = true
	}
	for _, m := range regexp.MustCompile(`var\(\s*(--[A-Za-z0-9_-]+)`).FindAllStringSubmatch(themeCSS, -1) {
		if tokens[m[1]] {
			t.Errorf("themeCSS uses the theme token %s", m[1])
		}
	}
	if !strings.Contains(themeCSS, "prefers-color-scheme: dark") {
		t.Error("themeCSS does not follow prefers-color-scheme")
	}
}

// The page links the vendored stylesheet and script and Datastar's script, the ones the app
// serves (spec "Routes"), and subscribes to the reloads as of the current generation.
func TestThemePage_Links(t *testing.T) {
	for _, want := range []string{
		`<link rel="stylesheet" href="/static/al/al.css">`,
		`<script type="module" src="/static/al/al.js"></script>`,
		`<script type="module" src="` + datastar.ScriptPath + `"></script>`,
		`data-init="@get('/_dev/reload?b=` + themeGenMark + `', {openWhenHidden: true})"`,
	} {
		if !strings.Contains(themePage, want) {
			t.Errorf("themePage lacks %s", want)
		}
	}
	d, _ := themeServer(t)
	d.gen = "abcd1234-3"
	w := serve(t, d, themeReq("GET", "/_dev/theme", ""), nil)
	if body := w.Body.String(); !strings.Contains(body, "/_dev/reload?b=abcd1234-3'") || strings.Contains(body, themeGenMark) {
		t.Errorf("the page served does not subscribe as of the generation:\n%s", body)
	}
}
