// Package adapters implements the blog's capabilities in Go. `alder build` copies it into the
// generated module (build/blog/adapters), next to the app's package, which it imports as "app";
// the generated cmd/main.go serves the app with Caps().
package adapters

import (
	"context"
	"errors"
	"time"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// Caps are the implementations of the app's capabilities.
func Caps() app.Caps {
	return app.Caps{Auth__Login__Password: password{}, Markdown__Markdown: markdown{}}
}

// devAccount is one account the development stub accepts.
type devAccount struct {
	email, name, role string
}

// The two accounts the development stub accepts, both with the password "password", so the
// blog can be tried as each kind of user: author@example.com is an Author (it can open /admin
// and write posts), reader@example.com is a Reader (it can comment; /admin is a 404 for it).
var devAccounts = []devAccount{
	{email: "author@example.com", name: "Author", role: "Author"},
	{email: "reader@example.com", name: "Reader", role: "Reader"},
}

const devPassword = "password"

// password is a development stub of Auth.Login.Password: it accepts the two devAccounts with the
// password "password" and nobody else. Replace it before deploying the app.
type password struct{}

// Verify returns the id of the development account's User row for one of devAccounts and
// "password", and Nothing for anything else.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Password != devPassword {
		return rt.Nothing[string](), nil
	}
	for _, acc := range devAccounts {
		if args.Email == acc.email {
			id, err := devUser(ctx, acc)
			if err != nil {
				return rt.Nothing[string](), err
			}
			return rt.Just(id), nil
		}
	}
	return rt.Nothing[string](), nil
}

// devUser is the id of acc's User row, created on first use: a post's author refers to a User
// (`author: User.UserId @ref User`), so the id must name a row. It reads and writes in the
// running Task's transaction (db.StoreFrom, db.TxFrom).
func devUser(ctx context.Context, acc devAccount) (string, error) {
	st, tx := db.StoreFrom(ctx), db.TxFrom(ctx)
	if st == nil {
		return "", errors.New("adapters: the context carries no store")
	}
	var users db.TableDesc
	for _, t := range app.Tables {
		if t.Name == "User.User" {
			users = t
		}
	}
	table := &db.Table[struct{}]{Name: users.Name, Columns: users.Columns}
	rows, err := st.Query(ctx, tx, users.Name, users.Columns, db.Where(db.CEq("email", acc.email), db.From(table)).SQL())
	if err != nil {
		return "", err
	}
	vals := make([]any, len(users.Columns))
	for i, c := range users.Columns {
		switch c.Name {
		case users.ID:
			if len(rows) > 0 {
				return db.DecodeText(rows[0][i])
			}
		case "email":
			vals[i] = acc.email
		case "name":
			vals[i] = acc.name
		case "passwordHash":
			vals[i] = ""
		case "role":
			vals[i] = acc.role
		}
	}
	return st.Insert(ctx, tx, users.Name, users.Columns, users.ID, users.Created, users.Updated, vals, time.Now())
}
