// Package auth implements the app's capability Auth.Login.Password in Go, written by
// `alder new auth` (the Alder book, chapter 15): passwords are stored as PBKDF2-SHA256 hashes
// (Go's crypto/pbkdf2, 600000 iterations, a 16-byte random salt), in the text
//
//	pbkdf2-sha256$<iterations>$<salt>$<hash>
//
// with the salt and the hash in unpadded standard base64. Verify reads the iteration count from
// the stored text, so hashes made with another count keep working when the count changes.
package auth

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"alder/rt"
	"alder/rt/db"
	"app"
)

// LoginPassword is the adapter of Auth.Login.Password.
func LoginPassword() app.Cap__Auth__Login__Password { return password{} }

// password implements Auth.Login.Password with PBKDF2-SHA256.
type password struct{}

const (
	scheme     = "pbkdf2-sha256"
	iterations = 600000
	saltSize   = 16
	keySize    = 32
	// maxIterations bounds the count a stored text may ask for, so that a corrupt row cannot
	// make one login take minutes.
	maxIterations = 10_000_000
)

// Verify is the id of the User row whose email is args.Email when args.Password is its
// password, and Nothing for an unknown email, a wrong password or a row with no password (an
// empty passwordHash). A stored text that is not a hash this adapter made is an error, never
// a login.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	return verify(args.Email, args.Password, func(email string) (string, string, bool, error) { return lookup(ctx, email) })
}

// Hash is the text to store in passwordHash for the password args.Value: a new random salt
// each time, so two users with one password have different hashes.
func (password) Hash(ctx context.Context, args app.Args__Auth__Login__Password__Hash) (string, error) {
	return Hash(args.Value)
}

// verify is Verify with the row's lookup a function: the id and the stored hash of the row
// with that email, and whether there is one.
func verify(email, pw string, lookup func(email string) (id, stored string, found bool, err error)) (rt.Maybe[string], error) {
	id, stored, found, err := lookup(email)
	if err != nil {
		return rt.Nothing[string](), err
	}
	if !found || stored == "" {
		// derive anyway, so that the answer's timing does not tell which emails have accounts
		Check(dummy, pw)
		return rt.Nothing[string](), nil
	}
	ok, err := Check(stored, pw)
	if err != nil || !ok {
		return rt.Nothing[string](), err
	}
	return rt.Just(id), nil
}

// dummy is a well-formed hash no password is checked against for real.
var dummy = scheme + "$" + strconv.Itoa(iterations) + "$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Hash derives the stored text of password with a new random salt.
func Hash(pw string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return hashWith(pw, salt, iterations)
}

// hashWith derives the stored text of password with salt and iter iterations.
func hashWith(pw string, salt []byte, iter int) (string, error) {
	key, err := pbkdf2.Key(sha256.New, pw, salt, iter, keySize)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", scheme, iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Check reports whether password is the one stored was derived from, comparing in constant
// time with the iteration count stored names. A stored text that is not
// `pbkdf2-sha256$<iterations>$<salt>$<hash>` is an error.
func Check(stored, pw string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != scheme {
		return false, errors.New("auth: the stored password hash is not pbkdf2-sha256$<iterations>$<salt>$<hash>")
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > maxIterations {
		return false, fmt.Errorf("auth: the stored password hash has a bad iteration count %q", parts[1])
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false, errors.New("auth: the stored password hash has a bad salt")
	}
	want, err := enc.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false, errors.New("auth: the stored password hash has a bad hash")
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// lookup reads the User row whose email is email, in the running Task's transaction
// (db.StoreFrom, db.TxFrom): its id and its passwordHash.
func lookup(ctx context.Context, email string) (id, stored string, found bool, err error) {
	st, tx := db.StoreFrom(ctx), db.TxFrom(ctx)
	if st == nil {
		return "", "", false, errors.New("auth: the context carries no store")
	}
	var users db.TableDesc
	for _, t := range app.Tables {
		if t.Name == "User.User" {
			users = t
		}
	}
	if users.Name == "" {
		return "", "", false, errors.New("auth: the app has no table " + "User.User")
	}
	table := &db.Table[struct{}]{Name: users.Name, Columns: users.Columns}
	rows, err := st.Query(ctx, tx, users.Name, users.Columns, db.Where(db.CEq("email", email), db.From(table)).SQL())
	if err != nil || len(rows) == 0 {
		return "", "", false, err
	}
	for i, c := range users.Columns {
		switch c.Name {
		case users.ID:
			id, err = db.DecodeText(rows[0][i])
		case "passwordHash":
			stored, err = db.DecodeText(rows[0][i])
		}
		if err != nil {
			return "", "", false, err
		}
	}
	return id, stored, true, nil
}
