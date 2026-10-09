package rt

import "testing"

func TestNav(t *testing.T) {
	if n := Rerender(); n.Tag != NavRerender {
		t.Errorf("Rerender() = %+v", n)
	}
	if n := Redirect(Page{Path: "/x"}); n.Tag != NavRedirect || n.Page.Path != "/x" {
		t.Errorf("Redirect = %+v", n)
	}
	if n := Reject("no"); n.Tag != NavReject || n.Text != "no" {
		t.Errorf("Reject = %+v", n)
	}
}
