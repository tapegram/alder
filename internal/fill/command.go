package fill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// maxCommandOutput bounds what the command may write on standard output, and on standard
// error: past it, the attempt fails.
const maxCommandOutput = 1 << 20

// commandWaitDelay is how long Complete waits for the command's output after it exits or is
// stopped, when a process it started still holds the pipes.
var commandWaitDelay = time.Second

// Command is the command backend (ALDER_FILL_COMMAND): Cmd runs under `sh -c` with the
// conversation (Transcript) on standard input, and its standard output is the answer. A
// non-zero exit fails the attempt with its standard error. The command runs in a process group
// of its own, killed whole when the attempt's context ends and once the command has exited, so
// nothing it started outlives the attempt. `claude -p` works as is; a script is the tests' fake.
type Command struct {
	Cmd string
	// Reference, when set, is a file whose text goes before the system prompt
	// (ALDER_FILL_REFERENCE), as System puts it and `alder fill --prompt` prints it.
	Reference string
}

// capped is a buffer that refuses to grow past maxCommandOutput.
type capped struct {
	b    strings.Builder
	over bool
}

var errTooMuch = errors.New("too much output")

func (c *capped) Write(p []byte) (int, error) {
	if c.b.Len()+len(p) > maxCommandOutput {
		c.over = true
		return 0, errTooMuch
	}
	return c.b.Write(p)
}

func (c *Command) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	if c.Reference != "" {
		b, err := os.ReadFile(c.Reference)
		if err != nil {
			return "", fmt.Errorf("ALDER_FILL_REFERENCE: %v", err)
		}
		system = strings.TrimRight(string(b), "\n") + "\n\n" + system
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Cmd)
	cmd.Stdin = strings.NewReader(Transcript(system, messages))
	var out, errb capped
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.WaitDelay = commandWaitDelay
	ownGroup(cmd)
	err := cmd.Run()
	killGroup(cmd) // what the command left running in the background
	switch {
	case ctx.Err() != nil:
		return "", ctx.Err()
	case out.over || errb.over:
		return "", fmt.Errorf("ALDER_FILL_COMMAND wrote more than %d MB: stopped", maxCommandOutput>>20)
	case errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success():
		// it answered and exited; a process it left behind held the pipe, and is killed
	case err != nil:
		msg := strings.TrimSpace(errb.b.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("ALDER_FILL_COMMAND failed: %s", msg)
	}
	return out.b.String(), nil
}
