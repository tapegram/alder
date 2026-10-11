// Package app implements capability App.Clock (a module named App: its adapters are in
// adapters/app/, and the package may be named app too).
package app

import (
	"context"

	"app"
	"app/adapters/internal/dbx"
)

// Clock is the adapter of App.Clock.
func Clock() app.Cap__App__Clock { return clock{} }

type clock struct{}

func (clock) Now(context.Context) (int64, error) { return dbx.Answer(), nil }
