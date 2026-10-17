package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"alder/internal/migrate"
	"alder/rt/db"
)

// Plan is what a deploy will do to the target's database: the migrations of the build that the
// live database has not applied yet, read from the build's migrations/ (their headers: the
// steps and the DROPS DATA lines `alder migrate` wrote), placed by the live schema's hash.
type Plan struct {
	// Known is whether the target's schema was fetched: false for a Script target (it applies
	// the migrations itself) and on a first deploy, where every migration is pending.
	Known bool `json:"known"`
	// FirstDeploy: the target has no database yet, so nothing can be lost.
	FirstDeploy bool `json:"firstDeploy,omitempty"`
	// Live is the first 12 hex digits of the live schema's hash ("" when not Known).
	Live string `json:"live,omitempty"`
	// Build is the hash the build's migrations lead to ("" for an app without migrations).
	Build string `json:"build,omitempty"`
	// Migrations are the pending ones, in order.
	Migrations []PlanMigration `json:"migrations"`
	// NoMigrations: the app has no migrations/ (its tables are ensured at start).
	NoMigrations bool `json:"noMigrations,omitempty"`
}

// PlanMigration is one pending migration file.
type PlanMigration struct {
	Name      string   `json:"name"`
	Steps     []string `json:"steps"`
	DropsData []string `json:"dropsData,omitempty"`
}

// DropsData is every DROPS DATA line of the plan, each prefixed with its migration's name:
// "0004_drop_summary: column Post.summary". It is empty on a first deploy (there is no data).
func (p Plan) DropsData() []string {
	if p.FirstDeploy || !p.Known {
		return nil
	}
	var out []string
	for _, m := range p.Migrations {
		for _, d := range m.DropsData {
			out = append(out, m.Name+": "+d)
		}
	}
	return out
}

// Lines is the plan as the command prints it, one line each, without indentation.
func (p Plan) Lines() []string {
	var out []string
	switch {
	case p.NoMigrations:
		return []string{"no migrations: the app's tables are ensured when it starts"}
	case !p.Known && !p.FirstDeploy:
		out = append(out, "the target's schema is not known (the script applies the migrations): every migration is listed")
	case p.FirstDeploy:
		out = append(out, "first deploy: the database is created by every migration")
	default:
		if len(p.Migrations) == 0 {
			return []string{fmt.Sprintf("no pending migration: the live schema %s is the build's", p.Live)}
		}
		out = append(out, fmt.Sprintf("live schema %s → build schema %s", p.Live, p.Build))
	}
	for _, m := range p.Migrations {
		out = append(out, m.Name)
		for _, s := range m.Steps {
			out = append(out, "  "+s)
		}
		for _, d := range m.DropsData {
			switch {
			case p.FirstDeploy:
				out = append(out, "  DROPS DATA: "+d+" (nothing to lose yet: the database is new)")
			case !p.Known:
				out = append(out, "  DROPS DATA (if not applied yet): "+d)
			default:
				out = append(out, "  DROPS DATA: "+d)
			}
		}
	}
	return out
}

// MakePlan plans the build whose migrations/ is migrationsDir against the live schema snapshot
// live (a schema.json; haveLive false when the target has none: a first deploy, or a target
// that cannot tell, known false). A live schema that none of the build's migrations leads to
// is an error: the target runs a schema newer than the build (or one changed by hand).
func MakePlan(migrationsDir string, live []byte, haveLive, known bool) (Plan, error) {
	return MakePlanApplied(migrationsDir, live, haveLive, known, nil)
}

// MakePlanApplied is MakePlan knowing the names of the migrations the target applied (nil when
// it does not say). A migration that changes no table — `alder new silo --empty`'s, which moves a
// table into a silo (migrate.SiloMigration) — leads from a schema to the same one, so the live
// schema's hash cannot place it: one at the end of the build's chain at the live hash is pending
// when applied does not name it or, when applied is nil, when the build's snapshot puts in a silo
// a table the live one does not (its DROPS DATA then needs --allow-data-loss).
func MakePlanApplied(migrationsDir string, live []byte, haveLive, known bool, applied []string) (Plan, error) {
	p := Plan{Known: known && haveLive, FirstDeploy: known && !haveLive, Migrations: []PlanMigration{}}
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return p, err
	}
	sort.Strings(files)
	snapPath := filepath.Join(migrationsDir, "schema.json")
	if _, err := os.Stat(snapPath); err != nil && len(files) == 0 {
		p.NoMigrations = true
		return p, nil
	}
	if snap, err := migrate.ReadSnapshot(snapPath); err == nil {
		if h, err := snap.Hash(); err == nil {
			p.Build = h[:db.HashLen]
		}
	}
	type mig struct {
		PlanMigration
		from, to string
	}
	var all []mig
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return p, err
		}
		name, from, to, drops, err := migrate.ParseHeader(string(b))
		if err != nil {
			return p, fmt.Errorf("migration %s: %v", filepath.Base(f), err)
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(f), ".sql")
		}
		all = append(all, mig{PlanMigration{Name: name, Steps: stepLines(string(b)), DropsData: drops}, from, to})
	}
	start := 0
	if p.Known {
		var s migrate.Schema
		if err := json.Unmarshal(live, &s); err != nil {
			return p, fmt.Errorf("the target's schema.json does not read: %v", err)
		}
		h, err := s.Hash()
		if err != nil {
			return p, err
		}
		p.Live = h[:db.HashLen]
		switch {
		case p.Live == p.Build:
			start = len(all)
		case len(s.Tables) == 0: // a database without tables: every migration applies
			start = 0
		default:
			start = -1
			for i, m := range all {
				if m.to == p.Live {
					start = i + 1
				}
			}
			if start < 0 {
				return p, fmt.Errorf("the target's database is at schema %s, which none of this build's migrations leads to: the target runs a newer build's schema (or one changed outside migrations); deploy a build that has its migrations", p.Live)
			}
		}
	}
	if p.Known && start > 0 {
		var liveSnap migrate.Schema
		json.Unmarshal(live, &liveSnap)
		moved := false
		if snap, err := migrate.ReadSnapshot(snapPath); err == nil {
			for _, t := range snap.Tables {
				if t.Silo != "" && !slices.ContainsFunc(liveSnap.Tables, func(l migrate.Table) bool { return l.Name == t.Name && l.Silo == t.Silo }) {
					moved = true
				}
			}
		}
		for start > 0 && all[start-1].from == all[start-1].to && all[start-1].to == p.Live {
			if applied != nil && slices.Contains(applied, all[start-1].Name) || applied == nil && !moved {
				break
			}
			start--
		}
	}
	for _, m := range all[start:] {
		p.Migrations = append(p.Migrations, m.PlanMigration)
	}
	return p, nil
}

// stepLines are the `-- step: ` lines of a migration file's header, without the prefix.
func stepLines(text string) []string {
	out := []string{}
	for _, l := range strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(l, "--") {
			break
		}
		if s, ok := strings.CutPrefix(l, "-- step: "); ok {
			out = append(out, s)
		}
	}
	return out
}

// SchemaTables are the names of the tables in which two schema snapshots differ (present in
// one only, or declared differently), sorted: the refusal of a rollback names them.
func SchemaTables(a, b []byte) []string {
	var sa, sb migrate.Schema
	json.Unmarshal(a, &sa)
	json.Unmarshal(b, &sb)
	byName := func(s migrate.Schema) map[string]string {
		m := map[string]string{}
		for _, t := range s.Tables {
			j, _ := json.Marshal(t)
			m[t.Name] = string(j)
		}
		return m
	}
	ma, mb := byName(sa), byName(sb)
	set := map[string]bool{}
	for n, j := range ma {
		if mb[n] != j {
			set[n] = true
		}
	}
	for n := range mb {
		if _, ok := ma[n]; !ok {
			set[n] = true
		}
	}
	var out []string
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// SchemaHash is the 12-digit hash of a schema.json ("" when it does not read).
func SchemaHash(b []byte) string {
	var s migrate.Schema
	if json.Unmarshal(b, &s) != nil {
		return ""
	}
	h, err := s.Hash()
	if err != nil {
		return ""
	}
	return h[:db.HashLen]
}
