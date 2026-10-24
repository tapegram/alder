package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Script is the escape hatch: `Script "./deploy.sh"`. The gate and the artifact are the
// engine's; the script receives the artifact directory (absolute) and the environment's name as
// its two arguments, the `requires` names and ALDER_SECRET comma-separated in ALDER_REQUIRES, and runs from the
// app's root with the command's own environment (ALDER_ALLOW_DATA_LOSS=1 only with
// --allow-data-loss). Its exit code is the deploy's result. status,
// rollback and logs are not available for it (there is no state the command can read).
type Script struct {
	Path   string // as declared, relative to AppDir unless absolute
	AppDir string
	Stdout io.Writer
	Stderr io.Writer
	// AllowDataLoss (the run's --allow-data-loss) gives the script ALDER_ALLOW_DATA_LOSS=1, which
	// the binary needs to apply a migration that drops data; without it the variable is removed.
	AllowDataLoss bool
	// Env is the environment the script starts from (nil: this process's, os.Environ()); a
	// later NAME=value overrides an earlier one. cmd/alder adds ALDER_HOST for --host.
	Env []string
}

// ScriptUnavailable is the refusal of status, rollback, logs and restore for a Script target.
func ScriptUnavailable(cmd, env string) error {
	return fmt.Errorf("`alder %s` is not available for a Script target: environment %s is deployed by its script, which keeps its own state", cmd, env)
}

// Describe is "Script ./deploy.sh".
func (s Script) Describe() string { return "Script " + s.Path }

// Run runs the script for the artifact dir and environment env.
func (s Script) Run(ctx context.Context, dir, env string, requires []string) error {
	path := s.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.AppDir, path)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return refuse("", "the deploy script %s does not exist (environment %s is `Script %q`)", s.Path, env, s.Path)
	}
	cmd := exec.CommandContext(ctx, path, abs, env)
	cmd.Dir = s.AppDir
	base := s.Env
	if base == nil {
		base = os.Environ()
	}
	vars := []string{}
	for _, kv := range base {
		if !strings.HasPrefix(kv, "ALDER_ALLOW_DATA_LOSS=") {
			vars = append(vars, kv)
		}
	}
	vars = append(vars, "ALDER_REQUIRES="+strings.Join(requires, ","))
	if s.AllowDataLoss {
		vars = append(vars, "ALDER_ALLOW_DATA_LOSS=1")
	}
	cmd.Env = vars
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return refuse("", "the deploy script %s exited %d", s.Path, ee.ExitCode())
		}
		return refuse("", "the deploy script %s did not run: %v", s.Path, err)
	}
	return nil
}
