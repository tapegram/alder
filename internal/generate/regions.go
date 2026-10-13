package generate

import (
	"fmt"
	"strings"
)

// Owned regions: the parts of a generated file that `alder regen admin` rewrites. A region is
// the lines between a begin marker and an end marker, each alone on its line (indentation
// aside):
//
//	-- @admin begin columns
//	…the generator's lines…
//	-- @admin end
//
// Everything outside the regions is the author's: regen leaves it byte for byte as it is. The
// markers are comments, so a region may sit wherever a comment line may: between declarations,
// among a block's statements, in a record's fields, between the elements of a view. Regions do
// not nest, and a name is used once per file.

// RegionBegin and RegionEnd are the markers' text.
const (
	RegionBegin = "-- @admin begin "
	RegionEnd   = "-- @admin end"
)

// regionSpan is one region of a file: its name, and the lines (0-based) of its markers.
type regionSpan struct {
	name       string
	begin, end int
}

// parseRegions finds the regions of src, refusing a marker without its pair, a region begun
// inside another, a begin marker without a name, and a name used twice.
func parseRegions(src string) ([]regionSpan, error) {
	var out []regionSpan
	open := -1
	name := ""
	seen := map[string]int{}
	for i, l := range strings.Split(src, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t+" ", RegionBegin) && strings.TrimSpace(strings.TrimPrefix(t+" ", RegionBegin)) == "":
			return nil, fmt.Errorf("the begin marker at line %d has no region name: write `%s<name>`", i+1, RegionBegin)
		case strings.HasPrefix(t, RegionBegin):
			n := strings.TrimSpace(strings.TrimPrefix(t, RegionBegin))
			if open >= 0 {
				return nil, fmt.Errorf("region `%s` has no end marker (region `%s` begins at line %d, inside it)", name, n, i+1)
			}
			if at, dup := seen[n]; dup {
				return nil, fmt.Errorf("region `%s` begins twice, at lines %d and %d", n, at, i+1)
			}
			seen[n] = i + 1
			open, name = i, n
		case t == RegionEnd:
			if open < 0 {
				return nil, fmt.Errorf("the end marker at line %d has no begin marker", i+1)
			}
			out = append(out, regionSpan{name, open, i})
			open = -1
		}
	}
	if open >= 0 {
		return nil, fmt.Errorf("region `%s` has no end marker", name)
	}
	return out, nil
}

// Regions is the region machinery, as the spec names it: Regions.Replace(src, name, body),
// Regions.Names(src), Regions.Check(src), Regions.Body(src, name).
var Regions regionOps

type regionOps struct{}

// Check reports a broken marker pair in src (nil when every region is whole).
func (regionOps) Check(src string) error {
	_, err := parseRegions(src)
	return err
}

// Names are the names of src's regions, in order.
func (regionOps) Names(src string) ([]string, error) {
	spans, err := parseRegions(src)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, s := range spans {
		names = append(names, s.name)
	}
	return names, nil
}

// Body is the text between the markers of region name in src ("" for an empty region),
// and whether src has that region.
func (regionOps) Body(src, name string) (string, bool, error) {
	spans, err := parseRegions(src)
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(src, "\n")
	for _, s := range spans {
		if s.name == name {
			return strings.Join(lines[s.begin+1:s.end], "\n"), true, nil
		}
	}
	return "", false, nil
}

// Replace is src with the lines of region name replaced by body (the lines as they are to
// read, indentation included; "" for none). The markers and every line outside the region stay
// as they are. A region src does not have is refused.
func (regionOps) Replace(src, name, body string) (string, error) {
	return ReplaceRegions(src, []Region{{Name: name, Body: body}})
}

// ReplaceRegions is src with each of regions' bodies in place of its region's lines: the hook
// Apply calls for an Edit with Replace. Every region named must be in src, once; src's other
// regions and the text outside them are left alone.
func ReplaceRegions(src string, regions []Region) (string, error) {
	spans, err := parseRegions(src)
	if err != nil {
		return "", err
	}
	byName := map[string]regionSpan{}
	for _, s := range spans {
		byName[s.name] = s
	}
	bodies := map[int]string{} // begin line → new body
	for _, r := range regions {
		s, ok := byName[r.Name]
		if !ok {
			return "", fmt.Errorf("there is no region `%s`", r.Name)
		}
		bodies[s.begin] = r.Body
	}
	lines := strings.Split(src, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		body, ok := bodies[i]
		if !ok {
			continue
		}
		if body != "" {
			out = append(out, strings.Split(body, "\n")...)
		}
		i = byNameEnd(spans, i) - 1 // past the old body, to the end marker
	}
	return strings.Join(out, "\n"), nil
}

// byNameEnd is the end marker's line of the region beginning at line begin.
func byNameEnd(spans []regionSpan, begin int) int {
	for _, s := range spans {
		if s.begin == begin {
			return s.end
		}
	}
	return begin + 1
}
