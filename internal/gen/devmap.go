package gen

import (
	"encoding/json"
	"sort"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// devMap is the app's map for the dev server's /_dev page, as compact JSON: the pages (path,
// requires, roles, parameters, messages), the tables (columns and attributes), the capability
// names and the session's shape. The runtime adds what only it knows: the build id, the
// migrations applied and, per capability, whether an adapter or a fake is wired. Serve hands it
// to web.Options.DevMap; only the dev server (ALDER_DEV=1) ever serves it.
type devMap struct {
	Pages   []devPage  `json:"pages"`
	Tables  []devTable `json:"tables"`
	Caps    []string   `json:"caps"`
	Session []string   `json:"session"`
	AppMsgs []string   `json:"appMsgs,omitempty"`
}

type devPage struct {
	Module   string   `json:"module"`
	Path     string   `json:"path"`
	Requires string   `json:"requires,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Params   []string `json:"params,omitempty"`
	Msgs     []string `json:"msgs,omitempty"`
}

type devTable struct {
	Name     string            `json:"name"`
	Columns  []string          `json:"columns"`
	ID       string            `json:"id,omitempty"`
	Created  string            `json:"created,omitempty"`
	Updated  string            `json:"updated,omitempty"`
	Unique   []string          `json:"unique,omitempty"`
	Refs     map[string]string `json:"refs,omitempty"`
	OnDelete map[string]string `json:"onDelete,omitempty"`
	Owner    string            `json:"owner,omitempty"`
}

func devFields(r *ir.RecordType) []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, f := range r.Fields {
		out = append(out, f.Name+": "+types.String(f.Type))
	}
	return out
}

// devMapJSON is p's devMap, encoded (map keys sort, so the output is stable).
func devMapJSON(p *ir.Program) string {
	m := devMap{Pages: []devPage{}, Tables: []devTable{}, Caps: []string{}, Session: devFields(p.Session)}
	for _, pg := range p.Pages {
		d := devPage{Module: pg.Module, Path: pg.Path, Requires: strings.Join(pg.Requires, ", "), Roles: pg.Roles, Params: devFields(pg.Params)}
		for _, msg := range pg.Msgs {
			d.Msgs = append(d.Msgs, msg.Name)
		}
		m.Pages = append(m.Pages, d)
	}
	for _, tb := range p.Tables {
		m.Tables = append(m.Tables, devTable{
			Name: tb.Module + "." + tb.Name, Columns: devFields(tb.Record), ID: tb.IDField,
			Created: tb.Created, Updated: tb.Updated, Unique: tb.Unique, Refs: tb.Refs, OnDelete: tb.OnDelete, Owner: tb.Owner(),
		})
	}
	for _, c := range p.Caps {
		m.Caps = append(m.Caps, c.Module+"."+c.Name)
	}
	for _, msg := range p.AppMsgs {
		m.AppMsgs = append(m.AppMsgs, msg.Name)
	}
	sort.Strings(m.Caps)
	b, err := json.Marshal(m)
	if err != nil {
		panic("gen: dev map: " + err.Error())
	}
	return string(b)
}
