// Package auth: Auth.Login's Password and Auth.Signup's Password share adapters/auth/.
package auth

import "app"

func LoginPassword() app.Cap__Auth__Login__Password { return password{} }

func SignupPassword() app.Cap__Auth__Signup__Password { return password{} }

type password struct{}
