package auth

import (
	"context"
	"testing"

	"app"
)

// Outside dev mode (ALDER_DEV is not "1") the stub refuses every login, the dev accounts too,
// before it looks for a store; in dev mode it goes on to find the account's User row (here,
// with no store in the context, that is an error).
func TestPassword_DevModeOnly(t *testing.T) {
	args := app.Args__Auth__Login__Password__Verify{Email: "author@example.com", Password: "password"}
	t.Setenv("ALDER_DEV", "")
	if got, err := (password{}).Verify(context.Background(), args); err != nil || got.Ok {
		t.Fatalf("outside dev mode: %+v, %v; want Nothing", got, err)
	}
	t.Setenv("ALDER_DEV", "1")
	if _, err := (password{}).Verify(context.Background(), args); err == nil {
		t.Fatal("in dev mode: no error; want the missing store's")
	}
}
