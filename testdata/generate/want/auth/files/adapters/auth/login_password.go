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
	"time"

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
// a login. A hash made with fewer iterations than iterations is replaced, on a login that
// verifies, by one made now.
func (password) Verify(ctx context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	var r *row
	look := func(email string) (string, string, bool, error) {
		var err error
		r, err = lookup(ctx, email)
		if err != nil || r == nil {
			return "", "", false, err
		}
		return r.id, r.stored, true, nil
	}
	return verify(args.Email, args.Password, look, func(stored string) error { return r.rehash(ctx, stored) })
}

// Hash is the text to store in passwordHash for the password args.Value: a new random salt
// each time, so two users with one password have different hashes.
func (password) Hash(ctx context.Context, args app.Args__Auth__Login__Password__Hash) (string, error) {
	return Hash(args.Value)
}

// check is Check; a test replaces it to see which hashes verify derives.
var check = Check

// verify is Verify with the row's lookup a function (the id and the stored hash of the row
// with that email, and whether there is one) and the store's update of the hash another.
func verify(email, pw string, lookup func(email string) (id, stored string, found bool, err error), rehash func(stored string) error) (rt.Maybe[string], error) {
	id, stored, found, err := lookup(email)
	if err != nil {
		return rt.Nothing[string](), err
	}
	if !found || stored == "" {
		// derive anyway, so that the answer's timing does not tell which emails have accounts
		check(dummy, pw)
		return rt.Nothing[string](), nil
	}
	ok, err := check(stored, pw)
	if err != nil || !ok {
		return rt.Nothing[string](), err
	}
	if storedIterations(stored) < iterations { // made before the count was raised: upgrade it
		h, err := Hash(pw)
		if err == nil {
			err = rehash(h)
		}
		if err != nil {
			return rt.Nothing[string](), err
		}
	}
	return rt.Just(id), nil
}

// storedIterations is the iteration count a well-formed stored text names.
func storedIterations(stored string) int {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 {
		return 0
	}
	n, _ := strconv.Atoi(parts[1])
	return n
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
	if err != nil || len(want) < 16 {
		return false, errors.New("auth: the stored password hash has a bad hash (under 16 bytes)")
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// row is a User row as the store holds it: its id, its stored hash, and every column's
// value, to write it back with a new hash.
type row struct {
	id, stored string
	users      db.TableDesc
	vals       []any
	hashAt     int
}

// lookup reads the User row whose email is email, in the running Task's transaction
// (db.StoreFrom, db.TxFrom); nil when there is none.
func lookup(ctx context.Context, email string) (*row, error) {
	st, tx := db.StoreFrom(ctx), db.TxFrom(ctx)
	if st == nil {
		return nil, errors.New("auth: the context carries no store")
	}
	var users db.TableDesc
	for _, t := range app.Tables {
		if t.Name == "User.User" {
			users = t
		}
	}
	if users.Name == "" {
		return nil, errors.New("auth: the app has no table " + "User.User")
	}
	table := &db.Table[struct{}]{Name: users.Name, Columns: users.Columns}
	rows, err := st.Query(ctx, tx, users.Name, users.Columns, db.Where(db.CEq("email", email), db.From(table)).SQL())
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	r := &row{users: users, vals: rows[0], hashAt: -1}
	for i, c := range users.Columns {
		switch c.Name {
		case users.ID:
			r.id, err = db.DecodeText(rows[0][i])
		case "passwordHash":
			r.stored, err = db.DecodeText(rows[0][i])
			r.hashAt = i
		}
		if err != nil {
			return nil, err
		}
	}
	if r.hashAt < 0 {
		return nil, errors.New("auth: the table " + "User.User" + " has no column " + "passwordHash")
	}
	return r, nil
}

// rehash stores a new hash in r's row, in the running Task's transaction.
func (r *row) rehash(ctx context.Context, stored string) error {
	vals := append([]any(nil), r.vals...)
	vals[r.hashAt] = stored
	return db.StoreFrom(ctx).Update(ctx, db.TxFrom(ctx), r.users.Name, r.users.Columns, r.users.ID, r.users.Updated, r.id, vals, time.Now())
}
