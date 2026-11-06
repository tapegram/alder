package web

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"alder/rt/html"
)

// CheckOptions is the options check of a write route's form (#160, plan call 11): the values
// posted on form (an Alder form's name, "Todos.MoveTo") by the request key (the write's
// rt.RouteKey, which the form that sent it carries) checked against the page they were posted
// from, sh (Request.RenderShown: Alder-Page, else Referer; nil when no GET route matches it). routes are the Patterns of the GET
// routes whose view renders a `<Select options>` of form (ir.Route.OptionRoutes).
//
// When sh is one of them and shows a <form> sending key with such selects (html.Offers), errs
// are the fields whose value none of its selects offered ("must be one of the options",
// MsgOption): the write answers them as a form that did not decode. Otherwise the post did not
// come from a page that offers its options — another page, one no GET route matches (another
// app, a stale build), one that no longer renders (a 404, a redirect), or one without that form
// (its row is gone) — and bad is a 400 naming why (BadRequestResponse: logged at warn).
func (rq *Request) CheckOptions(sh *Shown, routes []string, form, key string, values map[string]string) (errs []FieldError, bad *Response) {
	refuse := func(why string) (errs []FieldError, bad *Response) {
		r := BadRequestResponse(fmt.Errorf("form %s posted from %s, %s; it is checked against the options of %s", form, strconv.Quote(rq.ShownURL()), why, strings.Join(routes, ", ")))
		return nil, &r
	}
	switch {
	case sh == nil:
		return refuse("which is no page of this app")
	case !slices.Contains(routes, sh.Pattern):
		return refuse("a page (" + sh.Pattern + ") that does not offer its options")
	case sh.Status != http.StatusOK:
		return refuse("which answered status " + strconv.Itoa(sh.Status))
	}
	offers, found := html.Offers(sh.node, form, key)
	if !found {
		return refuse("which does not show the form that sent it")
	}
	fields := make([]string, 0, len(offers))
	for f := range offers {
		fields = append(fields, f)
	}
	slices.Sort(fields)
	for _, f := range fields {
		if o := offers[f]; !html.Offered(values[f], o.Choices, o.Blank) {
			errs = append(errs, FieldError{Field: f, Message: MsgOption})
		}
	}
	return errs, nil
}
