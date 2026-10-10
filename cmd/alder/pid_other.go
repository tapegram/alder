//go:build !unix

package main

import "os"

// pidAlive reports whether a process with the given pid (> 0) exists.
func pidAlive(pid int) bool {
	if pid <= 0 { // kill(0, 0) would ask about the caller's process group
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
