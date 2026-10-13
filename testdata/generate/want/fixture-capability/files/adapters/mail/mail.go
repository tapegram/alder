// Package mail holds the Go adapter of capability Mail.Mail, written by
// `alder new capability`: each method returns an error until you write it (the Alder book,
// chapter 7, "Implementing a capability").
package mail

import (
	"context"
	"errors"

	"app"
)

// Mail is the adapter of Mail.Mail.
func Mail() app.Cap__Mail__Mail { return adapterMail{} }

// adapterMail implements Mail.Mail.
type adapterMail struct{}

// Send is Mail.send: not implemented yet.
func (adapterMail) Send(ctx context.Context, args app.Args__Mail__Mail__Send) (struct{}, error) {
	var zero struct{}
	return zero, errors.New("not implemented")
}
