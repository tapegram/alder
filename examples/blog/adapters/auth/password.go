// Package auth implements the blog's capability of module Auth.Login in Go: a capability of
// module Auth.… lives in adapters/auth/. `alder build` copies it into the generated module
// (build/blog/adapters/auth), next to the app's package, which it imports as "app"; the
// generated package caps (build/blog/caps) wires LoginPassword into the app.
package auth

import (
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// LoginPassword is the adapter of Auth.Login.Password.
func LoginPassword() app.Cap__Auth__Login__Password { return password{} }

// devAccount is one account the development stub accepts.
type devAccount struct {
	email, name, role string
}

// The three accounts the development stub accepts, all with the password "password", so the
// blog can be tried as each kind of user: author@example.com is an Author (it can open /write
// and write posts), reader@example.com is a Reader (it can comment; /write and /admin are a 404
// for it), admin@example.com is an Admin (it can open the generated admin, /admin; book 15).
var devAccounts = []devAccount{
	{email: "author@example.com", name: "Author", role: "Author"},
	{email: "reader@example.com", name: "Reader", role: "Reader"},
	{email: "admin@example.com", name: "Admin", role: "Admin"},
}

const devPassword = "password"

// password is a development stub of Auth.Login.Password: in dev mode (ALDER_DEV=1, which
// `alder run` sets) it accepts the three devAccounts with the password "password" and nobody else;
// outside dev mode it refuses every login. Replace it before deploying the app.
type password struct{}

// refusing logs, once, that the stub is refusing every login because the app is not in dev mode.
var refusing sync.Once

// devMode reports whether the app runs in dev mode (ALDER_DEV=1); when it does not, it logs
// (once) that every login is refused.
func devMode() bool {
	if os.Getenv("ALDER_DEV") == "1" {
		return true
	}
	refusing.Do(func() {
		log.Print("the dev Password stub refuses every login outside dev mode: replace it")
	})
	return false
}

// Verify returns the id of the development account's User row for one of devAccounts and
// "password" in dev mode, and Nothing for anything else.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if !devMode() || args.Password != devPassword {
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
		return "", errors.New("auth: the context carries no store")
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
