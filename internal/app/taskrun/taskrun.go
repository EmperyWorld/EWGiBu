// Package taskrun executes manual tasks from an active deployment.
package taskrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"iacpack/internal/app/apply"
	"iacpack/internal/app/verify"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/journal"
	"iacpack/internal/runtime/provider"
	"iacpack/internal/runtime/secrets"
	"iacpack/internal/runtime/state"
)

// Options configures task run.
type Options struct {
	Project    string
	Task       string
	Instance   string
	StateHome  string
	Yes        bool
	Provider   provider.ApplyOps
	BundlePath string // optional override; default from state active path if present
}

// Result is the task run outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run executes a manual task only.
func Run(ctx context.Context, opts Options) Result {
	var res Result
	if opts.Project == "" || opts.Task == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "task run requires project and task")}
		return finish(res)
	}
	instance := opts.Instance
	if instance == "" {
		instance = identity.DefaultInstance
	}
	if opts.Provider == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, "provider required")}
		return finish(res)
	}
	home := opts.StateHome
	if home == "" {
		home = state.ResolveHome(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	}
	paths := state.ForInstance(home, opts.Project, instance)
	raw, err := os.ReadFile(paths.State)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, "no active deployment"))
		return finish(res)
	}
	doc, err := state.ParseDocument(raw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return finish(res)
	}
	bundlePath := opts.BundlePath
	if bundlePath == "" && doc.Active != nil {
		bundlePath = doc.Active.Path
	}
	if bundlePath == "" {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"active state has no bundle path; pass the verified bundle via apply first with recorded path").
			WithHint("Re-apply with a bundle, or extend state with path in a later release"))
		return finish(res)
	}
	depRaw, _, diags := verify.ReadVerifiedFile(bundlePath, "deployment.json")
	res.Diagnostics = append(res.Diagnostics, diags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	dep, err := compile.UnmarshalDeployment(depRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return finish(res)
	}
	task, ok := dep.Tasks[opts.Task]
	if !ok {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "unknown task "+opts.Task))
		return finish(res)
	}
	if task.Rerun != "manual" {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("TASK.NOT_MANUAL",
			fmt.Sprintf("task %q rerun %q is not manual", opts.Task, task.Rerun)))
		return finish(res)
	}
	if !opts.Yes {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"manual task requires --yes confirmation"))
		return finish(res)
	}
	lock, err := state.AcquireLock(paths)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	defer lock.Release()
	js, err := journal.Open(paths.Journal)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	defer js.Close()
	runID := journal.NewRunID(time.Now().UnixNano())
	aid := "action:task/" + opts.Task
	_ = js.Transition(runID, aid, false, journal.StatePlanned, nil, "")
	_ = js.Transition(runID, aid, false, journal.StateStarted, nil, "")
	composeFile := filepath.Join(paths.Rendered, "compose.yaml")
	if err := apply.RequireSecretFiles(paths.Secrets, apply.TaskSecretIDs(dep, opts.Task)); err != nil {
		_ = js.Transition(runID, aid, false, journal.StateFailed, nil, err.Error())
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(secrets.CodeSecretInvalid, err.Error()))
		return finish(res)
	}
	if err := opts.Provider.RunTask(ctx, doc.ProviderSlug, composeFile, opts.Task, 5*time.Minute); err != nil {
		_ = js.Transition(runID, aid, false, journal.StateFailed, nil, err.Error())
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("TASK.FAILED", err.Error()))
		return finish(res)
	}
	_ = js.Transition(runID, aid, false, journal.StateSucceeded, nil, "")
	res.OK = true
	res.Model = map[string]any{"project": opts.Project, "task": opts.Task, "instance": instance, "runId": runID}
	res.Text = fmt.Sprintf("task %s completed\n", opts.Task)
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
