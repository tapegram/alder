package jobs

import (
	"context"
	"testing"
)

// An attempt that ends leaves a newer attempt of its id registered (a row re-queued at its
// lease's end and claimed again while the first attempt still ran): cutOff and the watcher still
// reach the newer one.
func TestBegin_EndKeepsNewerAttempt(t *testing.T) {
	p := &pool{}
	row := Row{ID: "a1b2c3d4"}
	_, endOld := p.begin(context.Background(), row)
	newCtx, endNew := p.begin(context.Background(), row)
	defer endNew()
	endOld()
	if _, ok := p.attempts.Load(row.ID); !ok {
		t.Fatal("the newer attempt left the watcher's registry")
	}
	if !cutOff(row.ID) {
		t.Fatal("cutOff: false for the newer attempt")
	}
	if context.Cause(newCtx) != errCancelled {
		t.Fatalf("the newer attempt's cause: %v", context.Cause(newCtx))
	}
	endNew()
	if cutOff(row.ID) {
		t.Fatal("cutOff: true after both attempts ended")
	}
}
