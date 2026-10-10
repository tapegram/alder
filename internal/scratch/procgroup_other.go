//go:build !unix

package scratch

import "os/exec"

// ownGroup: without process groups only the program itself is killed at the timeout.
func ownGroup(cmd *exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {}
