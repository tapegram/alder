// Package work implements capability Work.Gate: hold waits until its attempt is cancelled (the
// workers' stop gives up waiting), so a test can stop a worker with a job running.
package work

import (
	"context"

	"app"
)

// Gate is the adapter of Work.Gate.
func Gate() app.Cap__Work__Gate { return gate{} }

type gate struct{}

func (gate) Hold(ctx context.Context) (struct{}, error) {
	<-ctx.Done()
	return struct{}{}, ctx.Err()
}
