package rt

import "testing"

// A route's `==` is its URL's: a parameter at its default (Omit) equals the route read back
// from the URL that leaves it out, inside a Response or a record too; Key agrees, and so does
// Show.
func TestRoute_AlderEqualAtDefaults(t *testing.T) {
	built := Route{Path: "/", Query: []KV{{Key: "filter", Value: "All", Omit: true}}}
	read := Route{Path: "/"}
	other := Route{Path: "/", Query: []KV{{Key: "filter", Value: "Active"}}}
	if !Eq(built, read) || Eq(built, other) {
		t.Fatalf("Eq(built, read) = %v, Eq(built, other) = %v", Eq(built, read), Eq(built, other))
	}
	if !Eq(Redirect(built), Redirect(read)) || Eq(Redirect(built), Redirect(other)) {
		t.Fatal("a Response's route is compared as its URL")
	}
	type rec struct{ P Route }
	if !Eq(rec{built}, rec{read}) {
		t.Fatal("a record's route is compared as its URL")
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
