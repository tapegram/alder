package migrate

import (
	"slices"
	"testing"

	"alder/rt/db"
)

// rt/db cannot import this package, so it repeats the naming of the @ref indexes
// (db.IndexStmts, which Ensure creates): both must agree, or a database made by Ensure and one
// made by migrations would differ in their indexes.
func TestIndexesAgreeWithEnsure(t *testing.T) {
	for _, f := range []string{"testdata/blog_schema.json", "testdata/todos_schema.json"} {
		s, err := ReadSnapshot(f)
		if err != nil {
			t.Fatal(err)
		}
		var descs []db.TableDesc
		for _, tb := range s.Tables {
			descs = append(descs, tb.Desc())
		}
		if got, want := db.IndexStmts(descs), s.Indexes(); !slices.Equal(got, want) {
			t.Fatalf("%s: db.IndexStmts = %q, Schema.Indexes = %q", f, got, want)
		}
		if f == "testdata/blog_schema.json" && len(s.Indexes()) == 0 {
			t.Fatal("the blog has @ref columns: no indexes compared")
		}
	}
}
