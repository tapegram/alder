//go:build !unix

package style

import (
	"os"
	"strconv"
)

// termWidth is COLUMNS if it parses, else 80; never below 40.
func termWidth(_ *os.File, getenv func(string) string) int {
	if c, err := strconv.Atoi(getenv("COLUMNS")); err == nil && c > 0 {
		return max(40, c)
	}
	return 80
}
