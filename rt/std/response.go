package std

import "alder/rt"

// The Response builders (#160). Each is rt's builder under the name StdCalls gives the stdlib
// function; the toast ones fix the kind.

// ResponseRedirect is `Response.redirect`.
func ResponseRedirect(r rt.Route) rt.Response { return rt.Redirect(r) }

// ResponseRender is `Response.render`.
func ResponseRender(r rt.Route) rt.Response { return rt.Render(r) }

// ResponseRefresh is `Response.refresh`.
func ResponseRefresh() rt.Response { return rt.Refresh() }

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
