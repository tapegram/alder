package generate

import (
	"strings"

	"alder/internal/check"
)

// newWriteRoute is NewRoute for --method POST|PUT|PATCH|DELETE: a module with a write route,
// which has an `update` answering `Response.refresh` and no `load` or `view`. It has no test: a
// write route is pressed from a page that has its button or form, which the module does not
// have (the test says so in a comment above `update`). A parameter that is a `{name}` segment of the path is the URL's, the others ride
// in the query string (--param names them all); --form F declares `form F = { }` and posts it.
func newWriteRoute(p *Program, o RouteOptions, route, method string) ([]Edit, error) {
	if o.List != "" || o.Show != "" || len(o.From) > 0 || o.Blank || o.File || o.ContentType != "" || o.By != "" || o.Example {
		return nil, Refusef("--method %s makes a write route, which has only `update`: --list, --show, --from, --blank, --file, --type, --by and --example belong to a GET route", method)
	}
	if o.Form == route {
		return nil, Refusef("`--form %s` is the route's own name: a form and a route cannot share one; name the form for what it holds, such as `%sForm`", o.Form, o.Form)
	}
	if o.Form != "" && !IsUpperIdent(o.Form) {
		return nil, Refusef("`--form %s` is not a form name: a form is named by a capitalised identifier, such as `Contact`", o.Form)
	}
	var params []FieldSpec
	names := map[string]bool{}
	for _, a := range o.Params {
		f, err := ParseField(a, false)
		if err != nil {
			return nil, Refusef("--param %v", err)
		}
		if f.Default != "" {
			return nil, Refusef("--param %s: a write route's parameter has no default; a route that needs an optional one gets it with `alder edit route %s add-param %s`", a, o.Module, a)
		}
		if names[f.Name] {
			return nil, Refusef("--param %s is given twice", f.Name)
		}
		names[f.Name] = true
		params = append(params, f)
	}
	for _, m := range pathParam.FindAllStringSubmatch(o.Path, -1) {
		if !names[m[1]] {
			return nil, Refusef("`%s` has the parameter `{%s}` but no `--param %s:<Type>`", o.Path, m[1], m[1])
		}
	}
	if strings.ContainsAny(pathParam.ReplaceAllString(o.Path, ""), "{}:") {
		return nil, Refusef("`%s` has a parameter: name its type with `--param <name>:<Type>` (`/posts/{id}/like --param id:Post.PostId`)", o.Path)
	}
	if why := check.PagePathProblem(o.Path); why != "" {
		return nil, Refusef("`%s` is not a route path: it %s; %s", o.Path, why, check.PagePathRule)
	}
	_, _, requires, err := routeGuard(p, o)
	if err != nil {
		return nil, err
	}
	o.Requires = requires
	imports := newImports(o.Module, p.Std)
	d := RouteData{Module: o.Module, Route: route, Method: method, Path: o.Path, Params: "{ }", FormName: o.Form}
	if o.Form != "" {
		d.Form = " form " + o.Form
	}
	if requires != "" {
		d.Requires = " requires " + requires
		if o.Role != "" {
			d.Requires += ", role " + o.Role
		}
	}
	if len(params) > 0 {
		var fs []string
		for _, f := range params {
			fs = append(fs, f.Source())
			typeImports(imports, f.Type, p)
		}
		d.Params = "{ " + strings.Join(fs, ", ") + " }"
	}
	d.Imports = imports.list()
	body, err := canonical(p, FileOf(o.Module), render("route_write.tmpl", d))
	if err != nil {
		return nil, err
	}
	return []Edit{{Path: FileOf(o.Module), Create: true, Body: body}}, nil
}
