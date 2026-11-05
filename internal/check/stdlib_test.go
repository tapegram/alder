package check

import (
	"strings"
	"testing"
)

// The Response builders ship with exactly the signatures of the routes plan (#160), and the
// builders it leaves out have no stdlib entry.
func TestStdlib_ResponseSignatures(t *testing.T) {
	app := "module App\n\n" +
		"redirect : Route -> Response\nredirect = Response.redirect\n\n" +
		"render : Route -> Response\nrender = Response.render\n\n" +
		"refresh : Response\nrefresh = Response.refresh\n\n" +
		"invalid : Text -> Response\ninvalid = Response.invalid\n\n" +
		"toast : Text -> Response -> Response\ntoast = Response.toast\n\n" +
		"success : Text -> Response -> Response\nsuccess = Response.success\n\n" +
		"warning : Text -> Response -> Response\nwarning = Response.warning\n\n" +
		"fail : Text -> Response -> Response\nfail = Response.error\n\n" +
		"external : Text -> Route\nexternal = Url.external\n\n" +
		"asPage : Route -> Page\nasPage r = r\n\n" +
		"error : Error -> Html Never\nerror err = <html><body></body></html>\n\nnotFound : Html Never\nnotFound = <html><body></body></html>\n"
	if _, errs := checkFiles(t, map[string]string{"App.alder": app}); len(errs) > 0 {
		t.Fatalf("signatures: %v", errs)
	}
	for _, name := range []string{"status", "header", "json", "file", "remove", "patch", "stream"} {
		src := "module App\n\nx : Int\nx = Response." + name + "\n\nerror : Error -> Html Never\nerror err = <html><body></body></html>\n\nnotFound : Html Never\nnotFound = <html><body></body></html>\n"
		if _, errs := checkFiles(t, map[string]string{"App.alder": src}); len(errs) == 0 {
			t.Errorf("Response.%s checks, but it is not shipped", name)
		}
	}
}

// Route and Response are Prelude types an app may not redeclare; Rendered and Refreshed are
// constructors of the test type Answer.
func TestStdlib_RouteResponseReserved(t *testing.T) {
	for _, decl := range []string{"type Route = { n: Int }", "type Response = { n: Int }", "type Rendered = Rendered | Other", "type Refreshed = Refreshed | Other"} {
		src := "module App\n\n" + decl + "\n\nerror : Error -> Html Never\nerror err = <html><body></body></html>\n\nnotFound : Html Never\nnotFound = <html><body></body></html>\n"
		_, errs := checkFiles(t, map[string]string{"App.alder": src})
		found := false
		for _, e := range errs {
			found = found || strings.HasPrefix(e, "E0012")
		}
		if !found {
			t.Errorf("%q: want E0012, got %v", decl, errs)
		}
	}
}
