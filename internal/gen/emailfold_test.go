package gen

import (
	"slices"
	"strings"
	"testing"

	"alder/internal/ir"
	"alder/rt/db"
	"alder/rt/std"
)

// The three Text.Email folds agree (#326): the compile-time fold of a literal (ir), the
// runtime constructor (std.TextFoldEmail) and rt/db's, which reads and writes a Text.Email
// column. rt/db cannot import rt/std, so this package, which can import all three, pins them.
func TestEmailFoldsAgree(t *testing.T) {
	for _, in := range []string{"", "ada@x.com", "Ada@X.com", "ADA@X.COM", "É@X.com", "é@x.com",
		"İstanbul@X.com", "ǅ@X.com", "ẞ@X.com", "ΣΟΣ@X.com", "a+b@x.com"} {
		lit, run, col := ir.FoldEmail(in), std.TextFoldEmail(in), db.FoldEmail(in)
		if lit != run || run != col {
			t.Errorf("fold %q: literal %q, std %q, db %q", in, lit, run, col)
		}
		if enc := db.EncodeEmail(in); enc != col {
			t.Errorf("EncodeEmail(%q) = %q, want %q", in, enc, col)
		}
		if dec, err := db.DecodeEmail(in); err != nil || dec != col {
			t.Errorf("DecodeEmail(%q) = %q, %v; want %q", in, dec, err, col)
		}
	}
}

const emailWriteApp = `module App

error : Error -> Html
error err = <p>error</p>

notFound : Html
notFound = <p>not found</p>
`

// emailWritePerson uses a Text.Email column (and a Maybe one) with every condition operator
// that takes an operand, and writes the columns from a capability's result.
const emailWritePerson = `module Person

type PersonId = brand Text

table Person {
  id: PersonId @id,
  email: Text.Email @unique,
  alt: Maybe Text.Email,
}

capability Directory {
  lookup : Text -> Query Text.Email,
}

conv : Text -> Maybe Text.Email
conv s = Text.toEmail s

add : Text.Email -> Task PersonId
add e = Db.insert Person { email: e, alt: Nothing }

addVia : Text -> Task PersonId
addVia key = {
  let e <- Directory.lookup key
  Db.insert Person { email: e, alt: Just e }
}

renameVia : PersonId -> Text -> Task Unit
renameVia id key = {
  let e <- Directory.lookup key
  Db.update Person id (\p -> { p | email: e })
}

byVia : Text -> Query (List Person)
byVia key = {
  let e <- Directory.lookup key
  Db.from Person |> Db.where (Person.email.eq e) |> Db.all
}

sEq : Text.Email -> Select Person
sEq e = Db.from Person |> Db.where (Person.email.eq e)

sNe : Text.Email -> Select Person
sNe e = Db.from Person |> Db.where (Person.email.ne e)

sLt : Text.Email -> Select Person
sLt e = Db.from Person |> Db.where (Person.email.lt e)

sLe : Text.Email -> Select Person
sLe e = Db.from Person |> Db.where (Person.email.le e)

sGt : Text.Email -> Select Person
sGt e = Db.from Person |> Db.where (Person.email.gt e)

sGe : Text.Email -> Select Person
sGe e = Db.from Person |> Db.where (Person.email.ge e)

sContains : Text -> Select Person
sContains s = Db.from Person |> Db.where (Person.email.contains s)

sStartsWith : Text -> Select Person
sStartsWith s = Db.from Person |> Db.where (Person.email.startsWith s)

sIn : List Text.Email -> Select Person
sIn es = Db.from Person |> Db.where (Person.email.in es)

sAltEq : Maybe Text.Email -> Select Person
sAltEq m = Db.from Person |> Db.where (Person.alt.eq m)

sAltIn : List (Maybe Text.Email) -> Select Person
sAltIn ms = Db.from Person |> Db.where (Person.alt.in ms)
`

// Every value written to a Text.Email column, and every operand compared with one, is folded
// by db.EncodeEmail (#326): an insert's and an update's column values, the operand of each
// condition operator (a Maybe column's too), and `.in`'s list. The operators are those of
// condFuncs plus `in`, so an operator added there fails here until this test (and its fold)
// covers it.
func TestGen_EmailColumnEncodesFolded(t *testing.T) {
	files := emitSourceWith(t, map[string]string{"App.alder": emailWriteApp, "Person.alder": emailWritePerson}, false)
	src := files["Person_alder.go"]
	const fold = "func(x string) any { return db.EncodeEmail(x) }"
	noOperand := []string{"and", "or", "not", "isNothing", "isJust"}
	ops := []string{"in"}
	for k := range condFuncs {
		if !slices.Contains(noOperand, k) {
			ops = append(ops, k)
		}
	}
	for _, op := range ops {
		if !strings.Contains(emailWritePerson, "Person.email."+op+" ") {
			t.Errorf("operator .%s has no case on a Text.Email column in this test: add one", op)
			continue
		}
		if op == "in" {
			contains(t, src, `db.CIn("email", db.EncodeList(es, `+fold+`))`)
			continue
		}
		contains(t, src, condFuncs[op]+`("email", db.EncodeEmail(`)
	}
	contains(t, src,
		`db.CEq("alt", db.EncodeMaybe(m, `+fold+`))`,
		`db.CIn("alt", db.EncodeList(ms, func(x rt.Maybe[string]) any {`, // gofmt breaks the line
		`return db.EncodeMaybe(x, `+fold+`)`,
		// insertvals, insertminevals and the descriptor's Encode (Db.update's row)
		`return []any{nil, db.EncodeEmail(r.Email), db.EncodeMaybe(r.Alt, `+fold+`)}`,
	)
	if n := strings.Count(string(src), `db.EncodeEmail(r.Email)`); n != 2 {
		t.Errorf("db.EncodeEmail(r.Email) appears %d times, want 2 (insertvals and Encode):\n%s", n, src)
	}
	vet(t, files)
}

// A Text.Email that reaches the app unfolded, from a Go capability adapter, is folded where it
// meets SQL (#326): `.eq` finds the stored lower-case row, an insert or an update of a case
// variant is refused by @unique, and each operator compares folded.
func TestGen_RunEmailFoldOnWrite(t *testing.T) {
	files := emitSourceWith(t, map[string]string{"App.alder": emailWriteApp, "Person.alder": emailWritePerson}, false)
	runGo(t, files, `package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
)

// directory is an identity adapter: it returns its key as the Text.Email, unfolded.
type directory struct{}

func (directory) Lookup(ctx context.Context, args Args__Person__Directory__Lookup) (string, error) {
	return args.Value, nil
}

var _ Cap__Person__Directory = directory{}

func TestEmailFoldOnWrite(t *testing.T) {
	ctx := context.Background()
	store, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Ensure(ctx, Tables, true); err != nil {
		t.Fatal(err)
	}
	env := &eff.Env{Ctx: ctx, Store: store, Caps: map[string]any{"Person.Directory": directory{}}, Now: time.Now}
	if _, err := eff.Run(env, eff.KTask, Person__add(Person__conv("Ada@X.com").Val)); err != nil {
		t.Fatal(err)
	}
	if _, err := eff.Run(env, eff.KTask, Person__add(Person__conv("É@X.com").Val)); err != nil {
		t.Fatal(err)
	}
	emails := func(name string, s db.Select[Person__Person]) string {
		t.Helper()
		rows, err := db.All(ctx, store, nil, db.OrderBy(db.Asc("email"), s))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var es []string
		for _, r := range rows {
			es = append(es, r.Email)
		}
		return strings.Join(es, ",")
	}

	// the adapter's Ada@X.com finds the stored ada@x.com
	if rows, err := eff.RunQuery(env, Person__byVia("Ada@X.com")); err != nil || len(rows) != 1 || rows[0].Email != "ada@x.com" {
		t.Errorf("byVia Ada@X.com = %+v %v, want the ada@x.com row", rows, err)
	}
	// and @unique refuses it as a second user
	if _, err := eff.Run(env, eff.KTask, Person__addVia("ADA@x.COM")); err == nil || !strings.Contains(err.Error(), "must be unique") {
		t.Errorf("addVia ADA@x.COM: %v, want a unique violation", err)
	}
	bob, err := eff.Run(env, eff.KTask, Person__addVia("Bob@Y.com"))
	if err != nil {
		t.Fatal(err)
	}
	// an update to a case variant of a stored email is refused too
	if _, err := eff.Run(env, eff.KTask, Person__renameVia(bob, "ADA@X.COM")); err == nil || !strings.Contains(err.Error(), "must be unique") {
		t.Errorf("renameVia ADA@X.COM: %v, want a unique violation", err)
	}
	if _, err := eff.Run(env, eff.KTask, Person__renameVia(bob, "Rob@Y.com")); err != nil {
		t.Fatal(err)
	}
	// every operator compares folded (the operands come in unfolded, as an adapter's would)
	for _, c := range []struct {
		name string
		sel  db.Select[Person__Person]
		want string
	}{
		{"eq", Person__sEq("ROB@Y.COM"), "rob@y.com"},
		{"ne", Person__sNe("ADA@X.COM"), "rob@y.com,é@x.com"},
		{"lt", Person__sLt("B"), "ada@x.com"},
		{"le", Person__sLe("ADA@X.COM"), "ada@x.com"},
		{"gt", Person__sGt("ROB@Y.COM"), "é@x.com"},
		{"ge", Person__sGe("ROB@Y.COM"), "rob@y.com,é@x.com"},
		{"contains", Person__sContains("É@"), "é@x.com"},
		{"startsWith", Person__sStartsWith("É"), "é@x.com"},
		{"in", Person__sIn([]string{"ADA@X.COM", "É@X.COM"}), "ada@x.com,é@x.com"},
		{"alt eq", Person__sAltEq(rt.Just("BOB@Y.COM")), "rob@y.com"},
		{"alt in", Person__sAltIn([]rt.Maybe[string]{rt.Just("Bob@y.com")}), "rob@y.com"},
	} {
		if got := emails(c.name, c.sel); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
`)
}
