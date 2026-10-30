//go:build !unix

package fill

import "os/exec"

// ownGroup does nothing where there are no process groups: the context's end kills sh alone.
func ownGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {}
