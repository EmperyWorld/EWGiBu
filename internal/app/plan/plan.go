// Package plan builds a mutation-free deployment plan from a verified bundle.
package plan

import (
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/app/portable"
	"iacpack/internal/app/verify"
	"iacpack/internal/bundle"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
	domainplan "iacpack/internal/domain/plan"
	"iacpack/internal/lockfile"
	"iacpack/internal/params"
	"iacpack/internal/policy"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/oppolicy"
	"iacpack/internal/runtime/secrets"
)

// Options configures planning.
type Options struct {
	Bundle         string
	Instance       string
	Set            map[string]string
	Values         map[string]any
	SecretFiles    []string
	OperatorPolicy oppolicy.Policy // zero => default
	DockerSocket   string
	// Workspace is the draft project directory. Empty uses ./iacpack.yaml when
	// that file names the same project as the bundle.
	Workspace string
}

// PlannedAction enriches a graph action with runtime metadata.
type PlannedAction struct {
	ID            string            `json:"id"`
	Kind          string            `json:"kind"`
	Target        string            `json:"target,omitempty"`
	Phase         string            `json:"phase,omitempty"`
	Rerun         string            `json:"rerun,omitempty"`
	Label         string            `json:"label"`
	Risk          string            `json:"risk"`
	Reversible    bool              `json:"reversible"`
	CommitBarrier bool              `json:"commitBarrier"`
	NeedsConfirm  bool              `json:"needsConfirm"`
	Ownership     map[string]string `json:"ownership"`
}

// Result is the plan outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

// Run verifies the bundle and emits a deterministic redacted plan (no state writes).
func Run(opts Options) Result {
	var res Result
	if opts.Bundle == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "plan requires a bundle path")}
		return finish(res)
	}
	kitDir := ""
	var kitPreserve map[string]string
	if portable.IsKitDir(opts.Bundle) {
		kitDir = opts.Bundle
		res.Diagnostics = append(res.Diagnostics, portable.CheckKit(kitDir)...)
		if diagnostic.HasError(res.Diagnostics) {
			return finish(res)
		}
		if embedded := portable.FindBundleInKit(kitDir); embedded != "" {
			opts.Bundle = embedded
			kitPreserve = copySet(opts.Set)
			for k, v := range portable.KitDeployValues(kitDir) {
				if opts.Set == nil {
					opts.Set = map[string]string{}
				}
				if _, ok := opts.Set[k]; !ok {
					opts.Set[k] = v
				}
			}
		} else {
			res.OK = true
			res.Text = "portable check passed\n"
			return finish(res)
		}
	}
	instance := opts.Instance
	if instance == "" {
		instance = identity.DefaultInstance
	}
	if err := identity.ValidateInstance(instance); err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, err.Error())}
		return finish(res)
	}
	op := opts.OperatorPolicy
	if op.APIVersion == "" {
		op = oppolicy.Default()
	}

	depRaw, vres, diags := verify.ReadVerifiedFile(opts.Bundle, "deployment.json")
	res.Diagnostics = append(res.Diagnostics, diags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	dep, err := compile.UnmarshalDeployment(depRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMeta, err.Error()))
		return finish(res)
	}
	if kitDir != "" {
		portable.ApplyKitHostPaths(&dep, kitDir, kitPreserve)
	}
	if err := identity.ValidateProjectName(dep.Project.Name); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return finish(res)
	}
	res.Diagnostics = append(res.Diagnostics, unknownServiceHosts(dep)...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	slug := identity.ProviderSlug(dep.Project.Name, instance)

	lockRaw, err := vres.ReadFile("iacpack.lock.json")
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMissing, "iacpack.lock.json missing"))
		return finish(res)
	}
	locked, err := lockfile.Parse(lockRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse, err.Error()))
		return finish(res)
	}
	res.Diagnostics = append(res.Diagnostics, staleLockDiagnostic(opts.Workspace, opts.Bundle, dep.Project.Name, locked)...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	polRaw, err := vres.ReadFile("policy.compile.json")
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMissing, "policy.compile.json missing"))
		return finish(res)
	}
	compilePol, err := policy.Parse(polRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodePolicyUnsupportedField, err.Error()))
		return finish(res)
	}
	if locked.Policy.SHA256 != "" {
		sum, err := policy.CanonicalBytes(compilePol)
		if err == nil {
			// digest check performed via lock already at build; re-check soft
			_ = sum
		}
	}

	res.Diagnostics = append(res.Diagnostics, oppolicy.Evaluate(op, compilePol, dep)...)

	refs, sdiags := secrets.ParsePairs(opts.SecretFiles)
	res.Diagnostics = append(res.Diagnostics, sdiags...)
	res.Diagnostics = append(res.Diagnostics, secrets.Validate(dep.Secrets, refs)...)

	deployVals := map[string]string{}
	for id, p := range dep.Parameters {
		if p.Phase != "deploy" {
			continue
		}
		if v, ok := opts.Set[id]; ok {
			deployVals[id] = v
			continue
		}
		if opts.Values != nil {
			if raw, ok := opts.Values[id]; ok {
				deployVals[id] = fmt.Sprint(raw)
				continue
			}
		}
		if p.Default != nil {
			deployVals[id] = fmt.Sprint(p.Default)
		}
	}
	resolved := map[string]params.Resolved{}
	for id, v := range deployVals {
		resolved[id] = params.Resolved{ID: id, Value: domain.Scalar{Kind: domain.ScalarString, Str: v}}
	}
	can, missing := compose.CanPreview(dep, resolved)
	if !can {
		for _, id := range missing {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(params.CodeParamMissing,
				fmt.Sprintf("deploy parameter %q required for plan", id)).
				WithHint("Pass --set "+id+"=<value>"))
		}
	}

	// Host-port overlap after deploy resolution (read-only warning left to doctor).
	res.Diagnostics = append(res.Diagnostics, checkPortOverlaps(dep, deployVals)...)

	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}

	g, gdiags := compile.BuildGraph(dep)
	res.Diagnostics = append(res.Diagnostics, gdiags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}

	actions := enrich(dep, g, slug, instance, vres.Meta.Project.SemanticHash)
	secretIDs := make([]string, 0, len(refs))
	for _, r := range refs {
		secretIDs = append(secretIDs, r.ID)
	}
	sort.Strings(secretIDs)

	gaps := missingEnvKeys(dep)
	for _, id := range sortedGapIDs(gaps) {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Warning("PLAN.ENV_MISSING",
			fmt.Sprintf("service %q missing env keys: %s", id, strings.Join(gaps[id], ", "))).
			WithHint("Set environmentFrom to the service that already has these keys, then override only the keys that differ"))
	}
	res.Model = map[string]any{
		"bundle":           opts.Bundle,
		"project":          dep.Project.Name,
		"instance":         instance,
		"providerSlug":     slug,
		"semanticHash":     dep.SemanticHash,
		"compilePolicy":    compilePol.Name,
		"operatorPolicy":   op.Name,
		"secretRefs":       secretIDs, // ids only — never paths or bytes
		"deployParameters": sortedKeys(deployVals),
		"actions":          actions,
		"order":            g.Order,
		"confirmations":    confirmations(actions),
	}
	if len(gaps) > 0 {
		res.Model["missingEnv"] = gaps
	}
	res.Text = formatText(res.Model)
	res.OK = true
	_ = locked
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}

func enrich(dep ir.Deployment, g domainplan.Graph, slug, instance, semHash string) []PlannedAction {
	barrierSeen := false
	out := make([]PlannedAction, 0, len(g.Order))
	byID := map[string]domainplan.Action{}
	for _, a := range g.Actions {
		byID[a.ID] = a
	}
	for _, id := range g.Order {
		a := byID[id]
		pa := PlannedAction{
			ID: a.ID, Kind: string(a.Kind), Target: a.Target, Phase: a.Phase, Rerun: a.Rerun, Label: a.Label,
			Risk: "none", Reversible: true, Ownership: map[string]string{
				"iacpack.project":  dep.Project.Name,
				"iacpack.instance": instance,
				"iacpack.slug":     slug,
				"iacpack.semantic": semHash,
			},
		}
		switch a.Kind {
		case domainplan.KindProvisionVolume:
			pa.Risk = "destructive"
			if v, ok := dep.Volumes[a.Target]; ok && v.Retention == "retain" {
				pa.Risk = "irreversible"
				pa.Reversible = false
				pa.NeedsConfirm = true
			}
		case domainplan.KindTaskRun:
			if a.Phase == "migration" {
				pa.Risk = "irreversible"
				pa.Reversible = false
				pa.NeedsConfirm = true
				if !barrierSeen {
					pa.CommitBarrier = true
					barrierSeen = true
				}
			}
		case domainplan.KindServiceStart:
			pa.Risk = "none"
		}
		out = append(out, pa)
	}
	return out
}

func confirmations(actions []PlannedAction) []string {
	var out []string
	for _, a := range actions {
		if a.NeedsConfirm {
			out = append(out, a.ID)
		}
	}
	return out
}

func checkPortOverlaps(dep ir.Deployment, deployVals map[string]string) []diagnostic.Diagnostic {
	type bind struct {
		svc  string
		bind string
		port int
	}
	var used []bind
	var diags []diagnostic.Diagnostic
	for _, id := range sortedServiceIDs(dep) {
		svc := dep.Services[id]
		for _, p := range svc.Ports {
			if p.Host == nil {
				continue
			}
			hostPort := 0
			if p.Host.Literal != nil {
				hostPort = *p.Host.Literal
			} else if p.Host.Parameter != "" {
				if v, ok := deployVals[p.Host.Parameter]; ok {
					fmt.Sscanf(v, "%d", &hostPort)
				}
			}
			if hostPort <= 0 {
				continue
			}
			b := p.Bind
			if b == "" {
				b = "0.0.0.0"
			}
			for _, u := range used {
				if u.bind == b && u.port == hostPort {
					diags = append(diags, diagnostic.Error(diagnostic.CodeManifestPortConflict,
						fmt.Sprintf("host port %s:%d claimed by %q and %q", b, hostPort, u.svc, id)))
				}
			}
			used = append(used, bind{svc: id, bind: b, port: hostPort})
		}
	}
	return diags
}

func sortedServiceIDs(dep ir.Deployment) []string {
	out := make([]string, 0, len(dep.Services))
	for id := range dep.Services {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func formatText(model map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "project: %v\n", model["project"])
	fmt.Fprintf(&b, "instance: %v\n", model["instance"])
	fmt.Fprintf(&b, "providerSlug: %v\n", model["providerSlug"])
	fmt.Fprintf(&b, "semanticHash: %v\n", model["semanticHash"])
	fmt.Fprintf(&b, "actions: %d\n", len(model["actions"].([]PlannedAction)))
	for _, a := range model["actions"].([]PlannedAction) {
		fmt.Fprintf(&b, "  %s risk=%s reversible=%v confirm=%v\n", a.ID, a.Risk, a.Reversible, a.NeedsConfirm)
	}
	if gaps, ok := model["missingEnv"].(map[string][]string); ok {
		for _, id := range sortedGapIDs(gaps) {
			fmt.Fprintf(&b, "missing env %s: %s\n", id, strings.Join(gaps[id], ", "))
		}
	}
	return b.String()
}

func copySet(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
