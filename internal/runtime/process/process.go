// Package process defines argv-based process execution for runtime adapters.
package process

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Spec is one process invocation (never a shell string).
type Spec struct {
	Path   string
	Args   []string // does not include Path
	Env    []string // already sanitized
	Dir    string
	Stdout io.Writer // optional live copy; Result.Stdout is still filled
	Stderr io.Writer
}

// Result is a completed process.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// Runner executes processes. Implementations must not invoke a shell.
type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// Call is one recorded invocation (for fakes/tests).
type Call struct {
	Spec Spec
}

// RecordingRunner records calls and returns scripted results. Mutation methods
// are not used in P07; callers assert zero unexpected calls.
type RecordingRunner struct {
	Calls  []Call
	Script func(spec Spec) (Result, error)
	Reject bool // if true, all Run return error
}

// Run implements Runner.
func (r *RecordingRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	_ = ctx
	if strings.Contains(spec.Path, " ") || looksLikeShell(spec) {
		return Result{}, fmt.Errorf("refusing shell-like process invocation")
	}
	r.Calls = append(r.Calls, Call{Spec: spec})
	if r.Reject {
		return Result{}, fmt.Errorf("runner rejected")
	}
	if r.Script != nil {
		return r.Script(spec)
	}
	return Result{ExitCode: 0}, nil
}

func looksLikeShell(spec Spec) bool {
	base := spec.Path
	if base == "/bin/sh" || base == "/bin/bash" || base == "sh" || base == "bash" {
		return true
	}
	for _, a := range spec.Args {
		if a == "-c" {
			return true
		}
	}
	return false
}

// AssertNoMutation reports whether any call looks like a Docker/Compose mutate.
func AssertNoMutation(calls []Call) error {
	mutate := []string{"up", "down", "create", "rm", "run", "push", "pull", "build", "start", "stop", "kill"}
	for _, c := range calls {
		joined := c.Spec.Path + " " + strings.Join(c.Spec.Args, " ")
		for _, m := range mutate {
			// allow "compose version" / "docker version" / "docker info"
			if containsArg(c.Spec.Args, m) && !containsArg(c.Spec.Args, "version") && !containsArg(c.Spec.Args, "info") {
				return fmt.Errorf("unexpected mutation-like argv: %s", joined)
			}
		}
	}
	return nil
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
