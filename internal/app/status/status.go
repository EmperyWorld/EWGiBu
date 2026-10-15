// Package status reports active deployment state without mutation.
package status

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/state"
)

// Options configures status.
type Options struct {
	Project   string
	Instance  string
	StateHome string
}

// Result is the status outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run reads state.json for a project/instance.
func Run(opts Options) Result {
	var res Result
	if opts.Project == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "status requires a project name")}
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
	raw, err := os.ReadFile(paths.State)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError,
			"no active state for project/instance").WithHint("Run apply first"))
		return finish(res)
	}
	doc, err := state.ParseDocument(raw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return finish(res)
	}
	sem := ""
	digest := ""
	if doc.Active != nil {
		sem = doc.Active.SemanticHash
		digest = doc.Active.Digest
	}
	runID, runStatus := "", ""
	if doc.LastRun != nil {
		runID = doc.LastRun.ID
		runStatus = doc.LastRun.Status
	}
	owned := make([]map[string]string, 0, len(doc.Owned))
	for _, o := range doc.Owned {
		owned = append(owned, map[string]string{"kind": o.Kind, "id": o.ID})
	}
	res.Model = map[string]any{
		"project": doc.Project, "instance": doc.Instance, "providerSlug": doc.ProviderSlug,
		"semanticHash": sem, "bundleDigest": digest,
		"lastRun": map[string]any{"id": runID, "status": runStatus},
		"owned":   owned, "recovery": doc.Recovery,
	}
	var b strings.Builder
	fmt.Fprintf(&b, "project: %s\ninstance: %s\nslug: %s\n", doc.Project, doc.Instance, doc.ProviderSlug)
	fmt.Fprintf(&b, "semanticHash: %s\nbundleDigest: %s\n", sem, digest)
	fmt.Fprintf(&b, "lastRun: %s (%s)\n", runID, runStatus)
	res.Text = b.String()
	res.OK = true
	_ = json.Marshal
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
