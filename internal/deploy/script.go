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
// its two arguments, the `requires` names comma-separated in ALDER_REQUIRES, and runs from the
// app's root with the command's own environment. Its exit code is the deploy's result. status,
// rollback and logs are not available for it (there is no state the command can read).
type Script struct {
	Path   string // as declared, relative to AppDir unless absolute
	AppDir string
	Stdout io.Writer
	Stderr io.Writer
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
	cmd.Env = append(os.Environ(), "ALDER_REQUIRES="+strings.Join(requires, ","))
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return refuse("", "the deploy script %s exited %d", s.Path, ee.ExitCode())
		}
		return refuse("", "the deploy script %s did not run: %v", s.Path, err)
	}
	return nil
}
