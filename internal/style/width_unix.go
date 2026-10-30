//go:build unix

package style

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// termWidth is f's width in columns by ioctl(TIOCGWINSZ), else COLUMNS, else 80; never below 40.
func termWidth(f *os.File, getenv func(string) string) int {
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno == 0 && ws.Col > 0 {
		return max(40, int(ws.Col))
	}
	if c, err := strconv.Atoi(getenv("COLUMNS")); err == nil && c > 0 {
		return max(40, c)
	}
	return 80
}
