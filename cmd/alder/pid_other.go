//go:build !unix

package main

import "os"

// pidAlive reports whether a process with the given pid exists.
func pidAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}
