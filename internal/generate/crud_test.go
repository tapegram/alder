package generate

import (
	"slices"
	"testing"
)

// TestAccessFound: Db.get loads a Maybe and matches on it, the statements between before the
// match; getMine and getScoped load the row itself, and the body follows directly.
func TestAccessFound(t *testing.T) {
	var a access
	post := &Table{Name: "Post"}
	between, body := []string{"let n <- x"}, []string{"Query.succeed row"}
	cases := []struct {
		get  string
		want []string
	}{
		{"Db.get", []string{"let found <- Db.get Post params.id", "let n <- x", "match found {", "  Just row -> {",
			"    Query.succeed row", "  },", "  Nothing -> Query.notFound,", "}"}},
		{"Db.getMine", []string{"let row <- Db.getMine Post params.id", "let n <- x", "Query.succeed row"}},
	}
	for _, c := range cases {
		if got := a.found(c.get, post, "params.id", between, body, "Query.notFound"); !slices.Equal(got, c.want) {
			t.Errorf("found(%s):\n got %q\nwant %q", c.get, got, c.want)
		}
	}
}
