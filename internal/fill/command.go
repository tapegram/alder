package fill

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Command is the command backend (ALDER_FILL_COMMAND): Cmd runs under `sh -c` with the
// conversation (Transcript) on standard input, and its standard output is the answer. A
// non-zero exit fails the attempt with its standard error. `claude -p` works as is; a script
// is the tests' fake.
type Command struct {
	Cmd string
}

func (c *Command) Complete(ctx context.Context, system string, messages []Message) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Cmd)
	cmd.Stdin = strings.NewReader(Transcript(system, messages))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("ALDER_FILL_COMMAND failed: %s", msg)
	}
	return out.String(), nil
}
