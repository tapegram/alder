//go:build unix

package main

import "syscall"

// pidAlive reports whether a process with the given pid (> 0) exists (signal 0: EPERM is a process
// of another user).
func pidAlive(pid int) bool {
	if pid <= 0 { // kill(0, 0) would ask about the caller's process group
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
