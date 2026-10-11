package auth

import "app"

// unexported: no constructor
func loginPassword() app.Cap__Auth__Login__Password { return nil }

// a method: no constructor
func (p password) SignupPassword() app.Cap__Auth__Signup__Password { return nil }

type password struct{}
