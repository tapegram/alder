//go:build unix

package main

import "syscall"

// pidAlive reports whether a process with the given pid exists (signal 0: EPERM is a process
// of another user).
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
