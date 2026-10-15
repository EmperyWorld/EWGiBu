// Package resume reconciles interrupted journaled runs.
package resume

import (
	"fmt"
	"os"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/journal"
	"iacpack/internal/runtime/state"
)

// Options configures resume.
type Options struct {
	Project   string
	Instance  string
	StateHome string
	DryRun    bool
}

// Result is the resume outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run inspects journal for unknown/started actions and either continues or refuses.
func Run(opts Options) Result {
	var res Result
	if opts.Project == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "resume requires a project")}
		return finish(res)
	}
	instance := opts.Instance
	if instance == "" {
		instance = identity.DefaultInstance
	}
	if err := identity.ValidateInstance(instance); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, err.Error())}
		return finish(res)
	}
	home := opts.StateHome
	if home == "" {
		home = state.ResolveHome(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	}
	paths := state.ForInstance(home, opts.Project, instance)
	if _, err := os.Stat(paths.Journal); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
			"no journal to resume").WithHint("Nothing to recover; use apply"))
		return finish(res)
	}
	js, err := journal.Open(paths.Journal)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	defer js.Close()
	last, err := js.LastByAction("")
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	unknown := journal.DetectUnknown(last)
	var reversible, irreversible []string
	for _, id := range unknown {
		r := last[id]
		if r.Reversible {
			reversible = append(reversible, id)
		} else {
			irreversible = append(irreversible, id)
		}
	}
	if len(irreversible) > 0 {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("RESUME.UNKNOWN_IRREVERSIBLE",
			fmt.Sprintf("refusing to auto-continue irreversible unknown actions: %s", strings.Join(irreversible, ", "))).
			WithHint("Probe the provider manually, then destroy/re-apply with operator confirmation"))
		res.Model = map[string]any{
			"project": opts.Project, "instance": instance,
			"unknownReversible": reversible, "unknownIrreversible": irreversible, "dryRun": opts.DryRun,
		}
		return finish(res)
	}
	if len(reversible) == 0 {
		res.OK = true
		res.Text = "nothing to resume\n"
		res.Model = map[string]any{"project": opts.Project, "instance": instance, "noop": true}
		return finish(res)
	}
	if opts.DryRun {
		res.OK = true
		res.Model = map[string]any{"project": opts.Project, "instance": instance, "dryRun": true, "continue": reversible}
		res.Text = "resume dry-run would continue: " + strings.Join(reversible, ", ") + "\n"
		return finish(res)
	}
	// Mark unknown reversible as failed-for-retry (operator continues with apply).
	for _, id := range reversible {
		_ = js.Transition(last[id].RunID, id, true, journal.StateUnknown, nil,
			"interrupted; marked unknown — re-run apply to reconcile")
	}
	res.OK = true
	res.Model = map[string]any{
		"project": opts.Project, "instance": instance, "reconciled": reversible,
		"hint": "re-run apply to continue",
	}
	res.Text = "marked interrupted reversible actions unknown; re-run apply\n"
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
