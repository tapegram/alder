//go:build browser

package main

import "testing"

// The theme page under `alder run` on the blog (spec "Verification", Browser): a change shows
// in the gallery at once with no rebuild, the switch flips to the dark scheme, clearing a token
// the committed theme sets shows alder-ui's default, Write edits App.alder and the rebuild
// reloads the page with an empty working diff, and the App tab's iframe follows the working
// theme across a navigation. ALDER_SHOTS_DIR keeps light and dark screenshots.
func TestThemeUI_Browser(t *testing.T) {
	t.Parallel()
	script := browserScript(t, "theme.cjs")
	runBrowserScript(t, script, startRunApp(t, "blog"))
}
