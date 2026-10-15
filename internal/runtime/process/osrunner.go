package process

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

// OSRunner executes argv arrays via os/exec (no shell).
type OSRunner struct{}

// Run implements Runner.
func (OSRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	if looksLikeShell(spec) {
		return Result{}, errShell
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if spec.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, spec.Stdout)
	}
	if spec.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, spec.Stderr)
	}
	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
	return res, nil
}

type shellError struct{}

func (shellError) Error() string { return "refusing shell-like process invocation" }

var errShell error = shellError{}
