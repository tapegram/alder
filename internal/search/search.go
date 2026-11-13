package search

import (
	"strings"

	"alder/internal/types"
)

// Options tune a search.
type Options struct {
	Limit  int    // 0 means 50; negative means no limit
	Source string // "" for every source, else only entries with this Source
}

// Hit is one search result.
type Hit struct {
	Entry *Entry
	Match string // "", or for type queries "exact", "general" or "reordered"
	Args  int    // produces mode: leading arguments still to supply
}

// Response is the answer to one query.
type Response struct {
	Query   string
	Mode    string // "name", "type" or "produces"
	Note    string
	Bad     bool  // a type query that does not parse
	Results []Hit // never nil
}

const defaultLimit = 50

// Search answers query against the index. It never fails: a query that cannot be answered gives no results.
func (ix *Index) Search(query string, o Options) Response {
	q := strings.TrimSpace(query)
	r := Response{Query: q, Mode: modeOf(q), Results: []Hit{}}
	var hits []Hit
	switch r.Mode {
	case "produces", "type":
		text := q
		if r.Mode == "produces" {
			text = q[1:]
		}
		if _, _, note, bad := ix.queryType(text); note != "" || bad {
			r.Note, r.Bad = note, bad
			return r
		}
		// matching is filled in by the type search
	default:
		hits = ix.searchName(q, o.Source)
		if len(hits) == 0 && looksLikeType(ix, q) {
			r.Note = "to search by type, start with `:`"
		}
	}
	limit := o.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	if len(hits) > 0 {
		r.Results = hits
	}
	return r
}

// modeOf picks the search mode of a trimmed query.
func modeOf(q string) string {
	switch {
	case strings.HasPrefix(q, ":"):
		return "produces"
	case strings.Contains(q, "->"):
		return "type"
	}
	return "name"
}

// looksLikeType is whether name query q reads as a type query: it converts, and it is more than
// a bare type variable (any lowercase word parses as one).
func looksLikeType(ix *Index, q string) bool {
	t, _, note, bad := ix.queryType(q)
	if note != "" || bad {
		return false
	}
	_, isVar := t.(*types.Param)
	return !isVar
}
