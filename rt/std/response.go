package std

import (
	"alder/rt"
	"alder/rt/html"
)

// The Response builders (#160). Each is rt's builder under the name StdCalls gives the stdlib
// function; the toast ones fix the kind.

// ResponseRedirect is `Response.redirect`.
func ResponseRedirect(r rt.Route) rt.Response { return rt.Redirect(r) }

// ResponseRender is `Response.render`.
func ResponseRender(r rt.Route) rt.Response { return rt.Render(r) }

// ResponseRefresh is `Response.refresh`.
func ResponseRefresh() rt.Response { return rt.Refresh() }

// ResponseReplace is `Response.replace` (#309): the views as the response's roots, flattened and
// checked when it is answered (rt/web.ReplaceRoots).
func ResponseReplace(views []html.Node) rt.Response {
	roots := make([]any, len(views))
	for i, v := range views {
		roots[i] = v
	}
	return rt.Replace(roots)
}

// ResponseRemove is `Response.remove`.
func ResponseRemove(id string, r rt.Response) rt.Response { return rt.WithRemove(id, r) }

// ResponseInvalid is `Response.invalid`.
func ResponseInvalid(text string) rt.Response { return rt.Invalid(text) }

// ResponseToast is `Response.toast`.
func ResponseToast(t string, r rt.Response) rt.Response { return rt.WithToast(rt.NoticeInfo, t, r) }

// ResponseSuccess is `Response.success`.
func ResponseSuccess(t string, r rt.Response) rt.Response {
	return rt.WithToast(rt.NoticeSuccess, t, r)
}

// ResponseWarning is `Response.warning`.
func ResponseWarning(t string, r rt.Response) rt.Response {
	return rt.WithToast(rt.NoticeWarning, t, r)
}

// ResponseError is `Response.error`.
func ResponseError(t string, r rt.Response) rt.Response { return rt.WithToast(rt.NoticeError, t, r) }
