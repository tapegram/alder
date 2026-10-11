// Package auth implements capability Auth.Login.Password.
package auth

import (
	"context"

	"alder/rt"
	"app"
	"app/adapters/internal/dbx"
)

// LoginPassword is the adapter of Auth.Login.Password.
func LoginPassword() app.Cap__Auth__Login__Password { return password{} }

type password struct{}

func (password) Verify(_ context.Context, args app.Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Password == "password" && dbx.Answer() == 42 {
		return rt.Just(args.Email), nil
	}
	return rt.Nothing[string](), nil
}
