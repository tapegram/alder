package gen

import (
	"encoding/json"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
	"alder/rt/db"
)

// devMap is the app's map for the dev server's /_dev page, as compact JSON: the pages (each GET
// route: its path, requires, roles, parameters), the tables (columns and attributes), the capability
// names and the session's shape. Each table also carries its SQL name, its columns' SQL names
// and storage types, and its silo; the map lists the app's silos. The runtime adds what only it
// knows: the build id, the migrations applied and, per capability, whether an adapter or a fake
// is wired. Serve hands it to web.Options.DevMap; only the dev server (ALDER_DEV=1) ever serves
// it.
type devMap struct {
	Pages   []devPage  `json:"pages"`
	Tables  []devTable `json:"tables"`
	Caps    []string   `json:"caps"`
	Session []string   `json:"session"`
	Silos   []devSilo  `json:"silos,omitempty"`
}

// devPage is a GET route; Module is its qualified name ("Todos.Index").
type devPage struct {
	Module   string   `json:"module"`
	Path     string   `json:"path"`
	Requires string   `json:"requires,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Params   []string `json:"params,omitempty"`
}

type devTable struct {
	Name     string            `json:"name"`
	SQL      string            `json:"sql"`
	Columns  []string          `json:"columns"`
	Cols     []devCol          `json:"cols"`
	ID       string            `json:"id,omitempty"`
	Created  string            `json:"created,omitempty"`
	Updated  string            `json:"updated,omitempty"`
	Unique   []string          `json:"unique,omitempty"`
	Refs     map[string]string `json:"refs,omitempty"`
	OnDelete map[string]string `json:"onDelete,omitempty"`
	Owner    string            `json:"owner,omitempty"`
	Silo     string            `json:"silo,omitempty"`
}

// devCol is a table's column: its Alder name and type (types.String) and its SQL name (the field's
// name, as columnLit gives it) and storage type, TEXT, INTEGER or REAL.
type devCol struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	SQL     string `json:"sql"`
	SQLType string `json:"sqlType"`
}

// devSilo is a `silo` declaration: its key, the table its values are ids of, its tables and the
// path of one value's database file relative to the shared database's directory. The placeholder
// {value} stands for db.SiloFileName(id), not the id itself: the id's text in lowercase base32hex
// (RFC 4648 section 7) without padding, then ".db", because ids are case-sensitive and a file
// system may not be. So the file of org id "a" is silos/org/c4.db.
type devSilo struct {
	Key    string   `json:"key"`
	Ref    string   `json:"ref"`
	Tables []string `json:"tables"`
	Path   string   `json:"path"`
}

// devSQLType is the storage type of a column of Alder type t, as db.ColType stores it: INTEGER for
// Int and Bool, REAL for Float and TEXT for everything else (Text, Date, Time and enums), a
// Maybe as its payload.
func devSQLType(t types.Type) string {
	if a := maybeArg(t); a != nil {
		t = a
	}
	if enumUnion(t) != nil {
		return "TEXT"
	}
	switch GoType(t) {
	case "int64", "bool":
		return "INTEGER"
	case "float64":
		return "REAL"
	}
	return "TEXT"
}

func devFields(r *ir.RecordType) []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, f := range r.Fields {
		s := f.Name + ": " + types.String(f.Type)
		if f.Wire != "" { // a route parameter's query key (#55)
			s += " as " + strconv.Quote(f.Wire)
		}
		out = append(out, s)
	}
	return out
}

// devMapJSON is p's devMap, encoded (map keys sort, so the output is stable).
func devMapJSON(p *ir.Program) string {
	m := devMap{Pages: []devPage{}, Tables: []devTable{}, Caps: []string{}, Session: devFields(p.Session)}
	for _, r := range p.Routes {
		if r.Method == "GET" {
			m.Pages = append(m.Pages, devPage{Module: routeKey(r), Path: r.Path, Requires: strings.Join(r.Requires, ", "), Roles: r.Roles, Params: devFields(r.Params)})
		}
	}
	siloOf := map[*ir.Table]string{}
	for _, k := range slices.Sorted(maps.Keys(p.Silos)) {
		s := devSilo{Key: k, Tables: []string{}, Path: "silos/" + k + "/{value}.db"}
		for _, tb := range p.Silos[k] {
			s.Tables = append(s.Tables, tb.Module+"."+tb.Name)
			siloOf[tb] = k
			if tb.Partition != nil && s.Ref == "" {
				s.Ref = tb.Refs[tb.Partition.Column]
			}
		}
		m.Silos = append(m.Silos, s)
	}
	for _, tb := range p.Tables {
		cols := []devCol{}
		if tb.Record != nil {
			for _, fd := range tb.Record.Fields {
				cols = append(cols, devCol{Name: fd.Name, Type: types.String(fd.Type), SQL: fd.Name, SQLType: devSQLType(fd.Type)})
			}
		}
		m.Tables = append(m.Tables, devTable{
			Name: tb.Module + "." + tb.Name, SQL: db.SQLName(tb.Module + "." + tb.Name), Cols: cols, Silo: siloOf[tb],
			Columns: devFields(tb.Record), ID: tb.IDField,
			Created: tb.Created, Updated: tb.Updated, Unique: tb.Unique, Refs: tb.Refs, OnDelete: tb.OnDelete, Owner: tb.Owner(),
		})
	}
	for _, c := range p.Caps {
		m.Caps = append(m.Caps, c.Module+"."+c.Name)
	}
	sort.Strings(m.Caps)
	b, err := json.Marshal(m)
	if err != nil {
		panic("gen: dev map: " + err.Error())
	}
	return string(b)
}
