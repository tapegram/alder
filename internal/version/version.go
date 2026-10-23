// Package version says which alder this binary is, from the build information the go command
// records in every binary (runtime/debug.ReadBuildInfo): `alder version` prints it and the
// generated app__alder.go records it in its header.
package version

import (
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// String is this binary's version: the module version of a released binary (release archive or
// Homebrew: "v0.1.0"); "devel+<commit>" (the first 12 hex digits, "-dirty" appended when the checkout had
// uncommitted changes) for a binary built in a checkout, whose module version is a pseudo-version
// or none; "(devel)" when the binary records neither.
func String() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return FromBuildInfo(bi)
}

// Go is the Go version this binary was built with, without the "go" prefix ("1.27.0").
func Go() string {
	return strings.TrimPrefix(runtime.Version(), "go")
}

// pseudo matches a pseudo-version's suffix (vX.Y.Z-yyyymmddhhmmss-abcdefabcdef, or the
// -0. / .0. forms) with the "+dirty" the go command appends to a modified checkout's.
var pseudo = regexp.MustCompile(`[-.]\d{14}-[0-9a-f]{12}(\+dirty)?$`)

// FromBuildInfo is String for the build information bi.
func FromBuildInfo(bi *debug.BuildInfo) string {
	v := bi.Main.Version
	if v != "" && v != "(devel)" && !pseudo.MatchString(v) && !strings.HasSuffix(v, "+dirty") {
		return v // a released binary (release archive or Homebrew)
	}
	var rev string
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "(devel)"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return "devel+" + rev
}

// Source says which git ref of alder's repository builds this binary (`alder deploy init` pins
// it in a Dockerfile). Ref is the release tag of a released binary or the full commit of a
// checkout build; "" when the binary records neither, or when the checkout had uncommitted
// changes (no ref builds those). Release is whether Ref is a tag; Dirty whether the checkout
// was modified.
type Source struct {
	Ref            string
	Release, Dirty bool
}

// SourceOf is this binary's Source.
func SourceOf() Source {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return Source{}
	}
	return SourceFromBuildInfo(bi)
}

// SourceFromBuildInfo is SourceOf for the build information bi.
func SourceFromBuildInfo(bi *debug.BuildInfo) Source {
	if v := FromBuildInfo(bi); !strings.HasPrefix(v, "devel+") && v != "(devel)" {
		return Source{Ref: v, Release: true}
	}
	var src Source
	rev := ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			src.Dirty = s.Value == "true"
		}
	}
	if !src.Dirty {
		src.Ref = rev
	}
	return src
}
