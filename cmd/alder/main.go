package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(runCheck(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: alder check [dir] [--json]") }

// runCheck is completed in Task 12.
func runCheck(args []string) int {
	fmt.Fprintln(os.Stderr, "alder check: not implemented")
	return 2
}
