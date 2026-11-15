package gen

import (
	"testing"

	"alder/internal/ir"
)

// secretField names a route's Text.Secret path parameters and form fields (a Maybe of one
// too), not its other parameters, nor a Secret query parameter (the dev panel shows path
// parameters and form fields; #254).
func TestSecretField(t *testing.T) {
	secret := &ir.Brand{Module: "Text", Name: "Secret"}
	email := &ir.Brand{Module: "Text", Name: "Email"}
	r := &ir.Route{Method: "POST", Path: "/doors/{door}/{id}",
		Params: &ir.RecordType{Fields: []ir.FieldT{{Name: "door", Brand: secret}, {Name: "id"}, {Name: "hint", Brand: secret}}},
		Form:   &ir.Form{Record: &ir.RecordType{Fields: []ir.FieldT{{Name: "email", Brand: email}, {Name: "pin", Brand: secret}}}},
	}
	if got, want := secretField(r), `Secret: []string{"door", "pin"}, `; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := secretField(&ir.Route{Method: "GET", Path: "/x/{id}", Params: &ir.RecordType{Fields: []ir.FieldT{{Name: "id", Brand: email}}}}); got != "" {
		t.Errorf("no secret: got %q", got)
	}
}
