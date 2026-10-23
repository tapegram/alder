package version

import (
	"runtime/debug"
	"testing"
)

// RF2: a released binary, a `go install …@version` one, a checkout build (pseudo-version or
// none, with VCS stamps) and a binary with no stamps each get a sensible string.
func TestFromBuildInfo(t *testing.T) {
	rev := []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.revision", Value: "71c46e8abcdef0123456789abcdef0123456789a"}, {Key: "vcs.modified", Value: "false"}}
	dirty := []debug.BuildSetting{{Key: "vcs.revision", Value: "71c46e8abcdef0123456789abcdef0123456789a"}, {Key: "vcs.modified", Value: "true"}}
	for _, c := range []struct {
		version  string
		settings []debug.BuildSetting
		want     string
	}{
		{"v0.1.0", nil, "v0.1.0"},              // go install …@v0.1.0
		{"v0.1.0", rev, "v0.1.0"},              // a release built from the tagged checkout
		{"v1.2.3-rc.1", nil, "v1.2.3-rc.1"},    // a prerelease is a version too
		{"(devel)", rev, "devel+71c46e8abcde"}, // a checkout build, no module version stamped
		{"", rev, "devel+71c46e8abcde"},        // (test binaries record none)
		{"v0.0.0-20261012093000-71c46e8abcde", rev, "devel+71c46e8abcde"},
		{"v0.0.0-20261012093000-71c46e8abcde+dirty", dirty, "devel+71c46e8abcde-dirty"},
		{"v0.1.1-0.20261012093000-71c46e8abcde", rev, "devel+71c46e8abcde"},
		{"(devel)", nil, "(devel)"}, // -buildvcs=false, or built outside a checkout
		{"", nil, "(devel)"},
	} {
		bi := &debug.BuildInfo{Main: debug.Module{Path: "alder", Version: c.version}, Settings: c.settings}
		if got := FromBuildInfo(bi); got != c.want {
			t.Errorf("FromBuildInfo(%q, %v) = %q, want %q", c.version, c.settings, got, c.want)
		}
	}
}

// SourceFromBuildInfo is what a Dockerfile can fetch: a release tag, or the full commit of a
// clean checkout; a modified checkout has no ref.
func TestSourceFromBuildInfo(t *testing.T) {
	full := "71c46e8abcdef0123456789abcdef0123456789a"
	rev := []debug.BuildSetting{{Key: "vcs.revision", Value: full}}
	dirty := []debug.BuildSetting{{Key: "vcs.revision", Value: full}, {Key: "vcs.modified", Value: "true"}}
	for _, c := range []struct {
		version  string
		settings []debug.BuildSetting
		want     Source
	}{
		{"v0.1.0", nil, Source{Ref: "v0.1.0", Release: true}},
		{"(devel)", rev, Source{Ref: full}},
		{"(devel)", dirty, Source{Dirty: true}},
		{"(devel)", nil, Source{}},
	} {
		bi := &debug.BuildInfo{Main: debug.Module{Path: "alder", Version: c.version}, Settings: c.settings}
		if got := SourceFromBuildInfo(bi); got != c.want {
			t.Errorf("SourceFromBuildInfo(%q, %v) = %+v, want %+v", c.version, c.settings, got, c.want)
		}
	}
}
