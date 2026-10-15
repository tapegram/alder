package generate

import "strings"

// afterLoginComment is the comment `new auth` writes in an app with no page yet.
const afterLoginComment = "the page ?afterLogin names (fill it, here and in Submit's arm, once the page exists)"

// EditAuth generates `alder edit auth --to <Page>`: the hole ?afterLogin of `Auth.Login`
// (`Submit`'s redirect and the test that logs in, which `new auth` leaves in an app with no
// page yet) replaced by the page, and the comment that names the hole. It refuses a name that
// is not a page, a page with parameters, and an app whose login has no such hole.
func EditAuth(p *Program, to string) ([]Edit, error) {
	if to == "" {
		return nil, Refusef("`alder edit auth` needs `--to <Page>`: the page a login goes to")
	}
	pg := p.PageNamed(to)
	if pg == nil {
		return nil, Refusef("`--to %s` is not a page of this app", to)
	}
	if hasRequiredParams(pg) {
		return nil, Refusef("`%s` has parameters: a login goes to a page without, name another with `--to <Page>`", to)
	}
	m := p.Modules["Auth.Login"]
	if m == nil || !strings.Contains(m.Source, "?afterLogin") {
		return nil, Refusef("`Auth.Login` has no hole ?afterLogin: the login goes to a page already (change `Submit`'s redirect with `alder fill --decl`)")
	}
	lit := pageLit(to, nil)
	body := strings.ReplaceAll(m.Source, afterLoginComment, lit)
	body = strings.ReplaceAll(body, "?afterLogin", lit)
	return []Edit{{Path: m.File, Body: body}}, nil
}
