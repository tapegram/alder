package generate

import "strings"

// afterLoginComment is the comment `new auth` writes in an app with no page yet.
const afterLoginComment = "the page ?afterLogin names (fill it, here and in SignIn.update, once the page exists)"

// afterLoginCommentPage is the comment an app made before routes has (PR 6 reworded it).
const afterLoginCommentPage = "the page ?afterLogin names (fill it, here and in Submit's arm, once the page exists)"

// EditAuth generates `alder edit auth --to <Page>`: the hole ?afterLogin of `Auth.Login`
// (`SignIn.update`'s redirect and the test that logs in, which `new auth` leaves in an app with
// no page yet) replaced by the page (a route module's GET route, a GET route `Module.Route`, or
// a page module written before routes), and the comment that names the hole. It refuses a name
// that is none of these, a page with parameters, and an app whose login has no such hole.
func EditAuth(p *Program, to string) ([]Edit, error) {
	if to == "" {
		return nil, Refusef("`alder edit auth` needs `--to <Page>`: the page a login goes to")
	}
	lit, hasParams, ok := loginTarget(p, to, "Auth.Login")
	if !ok {
		return nil, Refusef("`--to %s` is not a page of this app: name a route module (`Home`) or a GET route (`Posts.Index.Index`)", to)
	}
	if hasParams {
		return nil, Refusef("`%s` has parameters: a login goes to a page without, name another with `--to <Page>`", to)
	}
	m := p.Modules["Auth.Login"]
	if m == nil || !strings.Contains(m.Source, "?afterLogin") {
		return nil, Refusef("`Auth.Login` has no hole ?afterLogin: the login goes to a page already (change `SignIn.update`'s redirect with `alder fill --decl Auth.Login.SignIn.update -`)")
	}
	body := strings.ReplaceAll(m.Source, afterLoginComment, lit)
	body = strings.ReplaceAll(body, afterLoginCommentPage, lit)
	// the hole is an argument (`Response.redirect ?afterLogin`, as the formatter writes it): the
	// route literal, an application, takes parentheses there
	body = strings.ReplaceAll(body, "?afterLogin", "("+lit+")")
	return []Edit{{Path: m.File, Body: body}}, nil
}
