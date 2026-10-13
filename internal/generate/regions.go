package generate

import "errors"

// ErrRegionsUnsupported is ReplaceRegions' error until owned regions exist.
var ErrRegionsUnsupported = errors.New("owned regions are not supported yet")

// ReplaceRegions returns src with the owned regions named in regions replaced by their bodies:
// the hook Apply calls for an Edit with Replace. Owned regions (`-- @admin begin <name>` …
// `-- @admin end`, rewritten by `alder regen admin`) arrive with the admin generator (M12 Task
// 2), which implements this function; until then every such edit is refused with
// ErrRegionsUnsupported, and no ordinary generator makes one.
func ReplaceRegions(src string, regions []Region) (string, error) {
	return "", ErrRegionsUnsupported
}
