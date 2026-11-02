//go:build unix

package runner

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// killDelay is how long a cancelled alder has, after SIGTERM, to stop what it started (alder test
// kills its tests' own process group on SIGTERM) before its process group is killed.
var killDelay = 2 * time.Second

// group is cmd's process group, which the context's end stops whole.
type group struct {
	mu    sync.Mutex
	timer *time.Timer
	done  bool
}

// ownGroup runs cmd in a process group of its own. The context's end sends the group SIGTERM,
// then SIGKILL after killDelay if cmd has not been reaped by then: alder test starts the Go
// toolchain and the compiled tests, which would outlive alder alone.
func ownGroup(cmd *exec.Cmd) *group {
	g := &group{}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := cmd.Process.Pid
		g.mu.Lock()
		if !g.done {
			g.timer = time.AfterFunc(killDelay, func() {
				g.mu.Lock()
				defer g.mu.Unlock()
				if !g.done { // until reaped, cmd's pid, and so the group's id, is not reused
					syscall.Kill(-pgid, syscall.SIGKILL)
				}
			})
		}
		g.mu.Unlock()
		return syscall.Kill(-pgid, syscall.SIGTERM)
	}
	return g
}

// reaped records that cmd has been reaped, so its group is no longer killed.
func (g *group) reaped() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.done = true
	if g.timer != nil {
		g.timer.Stop()
	}
}
