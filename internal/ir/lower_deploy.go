package ir

import (
	"strconv"

	"alder/internal/syntax"
)

// lowerDeploy turns App's checked `deploy { … }` into the IR record. The checker has refused a
// malformed one (E0030); what does not fit the shape here is left out.
func lowerDeploy(d *syntax.DeployDecl) *Deploy {
	dp := &Deploy{Envs: map[string]Target{}, Proxy: "Caddy"}
	for _, f := range d.Fields {
		v := f.Value
		if v == nil {
			continue
		}
		switch f.Name {
		case "floor":
			if x, err := strconv.ParseFloat(v.Str, 64); err == nil && (v.Kind == "float" || v.Kind == "int") {
				dp.Floor, dp.FloorSet = x, true
			}
		case "keep":
			if x, err := strconv.Atoi(v.Str); err == nil && v.Kind == "int" {
				dp.Keep = x
			}
		case "checks":
			dp.Checks = deployStrings(v)
		case "requires":
			dp.Requires = deployStrings(v)
		case "backup":
			if v.Kind == "ctor" && v.Ctor == "S3" && v.Arg != nil {
				dp.Backup = &Backup{Kind: "s3", URL: v.Arg.Str}
			}
		case "proxy":
			if v.Kind == "ctor" {
				dp.Proxy = v.Ctor
			}
		default:
			if t, ok := lowerTarget(v); ok {
				dp.Envs[f.Name] = t
			}
		}
	}
	return dp
}

func deployStrings(v *syntax.DeployValue) []string {
	var out []string
	if v.Kind == "list" {
		for _, e := range v.List {
			out = append(out, e.Str)
		}
	}
	return out
}

func lowerTarget(v *syntax.DeployValue) (Target, bool) {
	if v.Kind != "ctor" {
		return Target{}, false
	}
	arg := ""
	if v.Arg != nil {
		arg = v.Arg.Str
	}
	var t Target
	switch v.Ctor {
	case "Ssh":
		t = Target{Kind: "ssh", Host: arg}
	case "Fly":
		t = Target{Kind: "fly", Region: arg}
	case "Script":
		t = Target{Kind: "script", ScriptPath: arg}
	default:
		return Target{}, false
	}
	for _, f := range v.Rec {
		if f.Value == nil {
			continue
		}
		s := f.Value.Str
		switch f.Name {
		case "domain":
			t.Domain = s
		case "path":
			t.Path = s
		case "user":
			t.User = s
		case "port":
			t.Port = s
		case "app":
			t.App = s
		case "database":
			t.Database = lowerDatabase(f.Value)
		}
	}
	return t, true
}

// lowerDatabase is `Turso "org" { group?, app, sample? }`, nil for anything else.
func lowerDatabase(v *syntax.DeployValue) *Database {
	if v.Kind != "ctor" || v.Ctor != "Turso" || v.Arg == nil {
		return nil
	}
	d := &Database{Kind: "turso", Org: v.Arg.Str, Group: "default"}
	for _, f := range v.Rec {
		if f.Value == nil {
			continue
		}
		switch f.Name {
		case "group":
			d.Group = f.Value.Str
		case "app":
			d.App = f.Value.Str
		case "sample":
			if n, err := strconv.Atoi(f.Value.Str); err == nil && f.Value.Kind == "int" {
				d.Sample = n
			}
		}
	}
	return d
}
