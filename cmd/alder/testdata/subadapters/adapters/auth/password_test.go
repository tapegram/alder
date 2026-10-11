package auth

import (
	"context"
	"os"
	"strings"
	"testing"

	"app"
)

// An adapter's test runs against the real app package, with its testdata/ copied too.
func TestVerify(t *testing.T) {
	b, err := os.ReadFile("testdata/user.txt")
	if err != nil {
		t.Fatal(err)
	}
	email := strings.TrimSpace(string(b))
	got, err := LoginPassword().Verify(context.Background(), app.Args__Auth__Login__Password__Verify{Email: email, Password: "password"})
	if err != nil || !got.Ok || got.Val != email {
		t.Fatalf("Verify = %+v, %v", got, err)
	}
}
