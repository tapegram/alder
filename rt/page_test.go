package rt

import "testing"

// A page's `==` is its URL's: a parameter at its default (Omit) equals the page read back from
// the URL that leaves it out, inside a Nav or a record too; Key agrees, and so does Show.
func TestPage_AlderEqual(t *testing.T) {
	built := Page{Path: "/", Query: []KV{{Key: "filter", Value: "All", Omit: true}}}
	read := Page{Path: "/"}
	other := Page{Path: "/", Query: []KV{{Key: "filter", Value: "Active"}}}
	if !Eq(built, read) || Eq(built, other) {
		t.Fatalf("Eq(built, read) = %v, Eq(built, other) = %v", Eq(built, read), Eq(built, other))
	}
	if !Eq(Redirect(built), Redirect(read)) || Eq(Redirect(built), Redirect(other)) {
		t.Fatal("a Nav's page is compared as its URL")
	}
	type rec struct{ P Page }
	if !Eq(rec{built}, rec{read}) {
		t.Fatal("a record's page is compared as its URL")
	}
	if Key(built) != Key(read) || Key(built) == Key(other) {
		t.Fatalf("keys %q %q %q", Key(built), Key(read), Key(other))
	}
	if Show(built) != Show(read) {
		t.Fatalf("Show %q, %q", Show(built), Show(read))
	}
}

type shown struct{ n int }

func (s shown) AlderShow() (string, bool) { return "Shown", false }

// A Shower prints itself, also as a constructor's argument.
func TestShow_Shower(t *testing.T) {
	if got := Show(Just(shown{})); got != "Just Shown" {
		t.Fatalf("Show = %q", got)
	}
}
