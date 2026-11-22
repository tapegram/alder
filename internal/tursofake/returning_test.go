package tursofake

import "testing"

// A write's RETURNING clause is found whatever whitespace (or none) is around it and in any
// case; the word in a string, a quoted name or a comment is not one.
func TestHasReturning(t *testing.T) {
	for _, c := range []struct {
		q    string
		want bool
	}{
		{`UPDATE "t" SET "a" = 1 RETURNING *`, true},
		{"UPDATE \"t\" SET \"a\" = 1\nRETURNING *", true},
		{"UPDATE \"t\" SET \"a\" = 1\n\treturning \"id\"", true},
		{"DELETE FROM \"t\"\r\nReturning\t\"id\"", true},
		{`INSERT INTO "t" ("a") VALUES (1)RETURNING "id"`, true},
		{`INSERT INTO "t" ("a") VALUES (1) RETURNING"id"`, true},
		{"UPDATE \"t\" SET \"a\" = 1 RETURNING\n*", true},
		{`UPDATE "t" SET "a" = 1`, false},
		{`UPDATE "t" SET "a" = ' RETURNING '`, false},
		{`UPDATE "t" SET "a" = 'it''s RETURNING'`, false},
		{`UPDATE "t" SET "returning" = 1`, false},
		{"UPDATE \"t\" SET \"a\" = 1 -- RETURNING *\n", false},
		{`UPDATE "t" SET "a" = 1 /* RETURNING * */`, false},
		{`UPDATE "t" SET "not_returning" = 1`, false},
		{`UPDATE "t" SET "a" = "returnings"`, false},
	} {
		if got := hasReturning(c.q); got != c.want {
			t.Errorf("hasReturning(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}
