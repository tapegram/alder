package generate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/names"
)

// ResourceOptions are the options of `alder new resource` and `alder regen resource`.
type ResourceOptions struct {
	Table    string
	Fields   []FieldSpec // create the table first when given
	Read     string      // "owner" | "signed-in" | "all"; "" = default
	Requires string      // as given; "" = default
	Module   string      // "" = PluralConcern(Table)
	Path     string      // "" = "/" + names.Collection(Table)

	explicitRequires bool // set when --requires was given: the header records it
}

// ResourceDir is the directory of the modules of the resource in module: "app/Posts".
func ResourceDir(module string) string {
	return "app/" + strings.ReplaceAll(module, ".", "/")
}

// resourceCheck fills the defaults and refuses what spec §1 refuses; it returns the table and
// the route keys (routeGuard's). A table that does not exist yet (its fields are given) comes
// back nil, with the module and path filled: Read and Requires wait for the table, so the check
// is made again on the program that has it.
func resourceCheck(p *Program, o ResourceOptions) (ResourceOptions, *Table, []string, error) {
	if o.Module == "" {
		o.Module = PluralConcern(o.Table)
	}
	if o.Path == "" {
		o.Path = "/" + names.Collection(o.Table)
	}
	var err error
	if o.Module, o.Path, err = pageOptionsCheck(o.Module, o.Path, "Posts", "/posts"); err != nil {
		return o, nil, nil, err
	}
	if o.Read != "" && !slices.Contains([]string{"owner", "signed-in", "all"}, o.Read) {
		return o, nil, nil, Refusef("`--read %s` is not `owner`, `signed-in` or `all`", o.Read)
	}
	t := p.TableNamed(o.Table)
	switch {
	case t == nil && len(o.Fields) == 0:
		return o, nil, nil, Refusef("there is no table `%s`: give its fields to create it (`alder new resource %s title:Text`) or make it first with `alder new table`", o.Table, o.Table)
	case t == nil:
		return o, nil, nil, nil
	case len(o.Fields) > 0:
		return o, nil, nil, Refusef("`%s` exists: change it with `alder edit table %s add-field …`, then `alder new resource %s`", t.Name, t.Name, t.Name)
	case t.Silo != "":
		return o, nil, nil, Refusef("`%s` lives in the `%s` silo: `alder new resource` writes pages over the shared store only (the admin shows siloed tables)", t.Name, t.Silo)
	}
	dir := ResourceDir(o.Module)
	if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(dir))); err == nil {
		return o, nil, nil, Refusef("`%s/` exists: `alder regen resource %s` updates it; to generate another, choose another `--module`", dir, t.Name)
	}
	if err := secretRefusal(t); err != nil {
		return o, nil, nil, err
	}
	hasUser := p.Session != nil && p.Session.User != nil
	if hasUser {
		for _, c := range t.Columns {
			if c.Ref != "" && c.Ref == p.Session.UserTable && !c.Owner && c.Name != t.PartCol && c.Required() && !c.HasDefault {
				return o, nil, nil, Refusef("`%s` is required and refers to users: give it a default or make it `Maybe`", c.Name)
			}
		}
	}

	owned := t.Owner != ""
	partitioned := t.PartKey != "" && !owned
	switch {
	case o.Read == "":
		switch {
		case owned || partitioned:
			o.Read = "owner"
		case hasUser:
			o.Read = "signed-in"
		default:
			o.Read = "all"
		}
	case o.Read == "owner" && !owned && !partitioned:
		return o, nil, nil, Refusef("`%s` has no `@owner` column to read by; add one (`alder edit table %s add-field owner:User.UserId@ref=User@owner`) or use `--read signed-in`", t.Name, t.Name)
	case o.Read != "owner" && partitioned:
		return o, nil, nil, Refusef("rows of `%s` are partitioned by `%s` and read within it; `--read` applies to `@owner` tables", t.Name, t.PartKey)
	case o.Read == "all" && owned && !hasUser:
		return o, nil, nil, Refusef("`%s` is owned (`@owner`): its pages need `--requires user`", t.Name)
	}

	o.explicitRequires = o.Requires != ""
	ro := RouteOptions{Requires: o.Requires}
	if !o.explicitRequires {
		switch {
		case t.PartKey != "":
			ro.Requires = t.PartKey
		case hasUser:
			ro.Requires = "user"
		}
	}
	keys, _, requires, err := routeGuard(p, ro)
	if err != nil {
		return o, nil, nil, err
	}
	if t.PartKey != "" && !slices.Contains(keys, t.PartKey) {
		if owned {
			return o, nil, nil, Refusef("`%s` is owned (`@owner`): its pages need `--requires user`", t.Name)
		}
		return o, nil, nil, Refusef("`%s` is partitioned by `%s`: its pages need `--requires %s`", t.Name, t.PartKey, t.PartKey)
	}
	if !o.explicitRequires {
		o.Requires = requires
	}
	return o, t, keys, nil
}
