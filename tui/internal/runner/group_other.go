//go:build !unix

package runner

import "os/exec"

// group does nothing where there are no process groups: the context's end kills alder alone.
type group struct{}

func ownGroup(cmd *exec.Cmd) *group { return &group{} }

func (*group) reaped() {}
