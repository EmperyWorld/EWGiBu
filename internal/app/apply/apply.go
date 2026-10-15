// Package apply performs journaled deployment of a verified bundle (mutating).
package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"iacpack/internal/app/plan"
	"iacpack/internal/app/portable"
	"iacpack/internal/app/verify"
	"iacpack/internal/bundle"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/identity"
	"iacpack/internal/runtime/journal"
	"iacpack/internal/runtime/oppolicy"
	"iacpack/internal/runtime/provider"
	"iacpack/internal/runtime/secrets"
	"iacpack/internal/runtime/state"
)

// Options configures apply.
type Options struct {
	Bundle         string
	Instance       string
	Set            map[string]string
	Values         map[string]any
	SecretFiles    []string
	OperatorPolicy oppolicy.Policy
	Yes            bool
	StateHome      string // test override; empty => XDG
	Provider       provider.ApplyOps
	AllowDevBuild  bool
	Workspace      string
}

// Result is the apply outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	Mutated     bool
	ExitHint    int // 6 preflight, 7 partial
}

// Run executes a journaled apply.
func Run(ctx context.Context, opts Options) Result {
	var res Result
	if opts.Bundle == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "apply requires a bundle path")}
		return finish(res)
	}
	kitDir := ""
	var kitPreserve map[string]string
	if portable.IsKitDir(opts.Bundle) {
		kitDir = opts.Bundle
		res.Diagnostics = append(res.Diagnostics, portable.CheckKit(kitDir)...)
		if diagnostic.HasError(res.Diagnostics) {
			res.ExitHint = 6
			return finish(res)
		}
		if embedded := portable.FindBundleInKit(kitDir); embedded != "" {
			opts.Bundle = embedded
		} else {
			res.OK = true
			res.Text = "portable check passed\n"
			res.Model = map[string]any{"portableCheck": true, "kit": kitDir}
			return finish(res)
		}
		kitPreserve = copySet(opts.Set)
		for k, v := range portable.KitDeployValues(kitDir) {
			if opts.Set == nil {
				opts.Set = map[string]string{}
			}
			if _, ok := opts.Set[k]; !ok {
				opts.Set[k] = v
			}
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
	if opts.Provider == nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInternal, "apply provider required")}
		return finish(res)
	}
	op := opts.OperatorPolicy
	if op.APIVersion == "" {
		op = oppolicy.Default()
	}

	// Plan first (verify + policy + secrets + params) — mutation-free.
	pres := plan.Run(plan.Options{
		Bundle: opts.Bundle, Instance: instance, Set: opts.Set, Values: opts.Values,
		SecretFiles: opts.SecretFiles, OperatorPolicy: op, Workspace: opts.Workspace,
	})
	res.Diagnostics = append(res.Diagnostics, pres.Diagnostics...)
	if !pres.OK {
		res.ExitHint = 6
		return finish(res)
	}

	depRaw, vres, diags := verify.ReadVerifiedFile(opts.Bundle, "deployment.json")
	res.Diagnostics = append(res.Diagnostics, diags...)
	if diagnostic.HasError(res.Diagnostics) {
		res.ExitHint = 6
		return finish(res)
	}
	dep, err := compile.UnmarshalDeployment(depRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMeta, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	if kitDir != "" {
		portable.ApplyKitHostPaths(&dep, kitDir, kitPreserve)
	}
	slug := identity.ProviderSlug(dep.Project.Name, instance)
	bundleSum := fileSHA(opts.Bundle)

	stateHome := opts.StateHome
	if stateHome == "" {
		stateHome = state.ResolveHome(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
	}
	paths := state.ForInstance(stateHome, dep.Project.Name, instance)
	deployVals := deployParamValues(dep, opts.Set, opts.Values)
	inputPath := filepath.Join(paths.Root, "task-inputs.json")
	prevInputs := loadTaskInputs(inputPath)
	migrationDrift := migrationInputDrift(dep, vres.Entries, deployVals, prevInputs)

	// Same-bundle re-apply is a no-op unless a oncePerDeployment task's
	// command, entrypoint, user, or mount files changed. A kit also noops
	// only when the filled host paths match the last successful apply.
	kitStamp := portable.KitHostStamp(kitDir)
	prevKitStamp := readKitHostStamp(paths.Root)
	hostStamp := hostRenderStamp()
	prevHostStamp := readRenderHostStamp(paths.Root)
	if existing, err := os.ReadFile(paths.State); err == nil {
		doc, err := state.ParseDocument(existing)
		if err == nil && doc.Active != nil && doc.Active.Digest == bundleSum && doc.Active.SemanticHash == dep.SemanticHash && !migrationDrift && kitStamp == prevKitStamp && hostStamp == prevHostStamp {
			res.OK = true
			res.Model = map[string]any{
				"project": dep.Project.Name, "instance": instance, "noop": true,
				"providerSlug": slug, "semanticHash": dep.SemanticHash,
			}
			res.Text = "noop: active bundle already matches\n"
			return finish(res)
		}
	}

	secretRefs, secretDiags := secrets.ParsePairs(opts.SecretFiles)
	res.Diagnostics = append(res.Diagnostics, secretDiags...)
	res.Diagnostics = append(res.Diagnostics, preflightDeployment(dep, deployVals, opts.AllowDevBuild, secretRefs, imagePresentFor(ctx, opts.Provider))...)
	if diagnostic.HasError(res.Diagnostics) {
		res.ExitHint = 6
		return finish(res)
	}
	if puller, ok := opts.Provider.(interface {
		PullImages(context.Context, []string) error
	}); ok {
		refs := missingPullRefs(dep, deployVals, imagePresentFor(ctx, opts.Provider))
		if len(refs) > 0 {
			if err := puller.PullImages(ctx, refs); err != nil {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(codeImagePull, err.Error()).
					WithHint("podman pull "+strings.Join(refs, " ")))
				res.ExitHint = 6
				return finish(res)
			}
		}
	}

	lock, err := state.AcquireLock(paths)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, "operation lock: "+err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	defer lock.Release()

	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	_ = state.EnsurePermissions(paths)

	js, err := journal.Open(paths.Journal)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	defer js.Close()
	runID := journal.NewRunID(time.Now().UnixNano())

	// Materialize secrets. Paths were checked before any state write.
	secMap := map[string]string{}
	for _, r := range secretRefs {
		secMap[r.ID] = r.Path
	}
	if err := provider.MaterializeSecrets(paths.Secrets, secMap); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(secrets.CodeSecretInvalid, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	if err := relabelContainerFiles(paths.Secrets); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}

	// Render compose into managed root
	lockRaw, _ := vres.ReadFile("iacpack.lock.json")
	locked, _ := lockfile.Parse(lockRaw)
	renderDep := dep
	renderDep.Services = cloneServiceResources(dep.Services)
	renderOpts := compose.RenderOptions{DeployValues: deployVals, Lock: locked, HostNVIDIADevices: hostNVIDIADevices(), SELinuxEnforcing: selinuxEnforcing()}
	if ids := devicesOnlyWithoutNodes(dep, renderOpts.HostNVIDIADevices); len(ids) > 0 {
		for _, id := range ids {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(codeApplyGPUUnavailable,
				fmt.Sprintf("service %q gpu: devices-only found no /dev/nvidia* nodes", id)))
		}
		res.ExitHint = 6
		return finish(res)
	}
	if podmanCountAllCDIMissing(opts.Provider, dep) {
		for _, id := range sortedGPUServices(dep) {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(codeApplyGPUUnavailable,
				fmt.Sprintf("service %q gpu: all needs CDI %s; devices without libcuda run on CPU", id, provider.NVIDIACountAllCDIDevice)).
				WithHint("Install nvidia-container-toolkit, or set gpu: devices-only only when the image supplies its own CUDA libraries"))
		}
		res.ExitHint = 6
		return finish(res)
	}
	dirs, err := imageWorkDirs(ctx, opts.Provider, dep)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	renderOpts.ImageWorkDirs = dirs
	yamlBytes, err := renderComposeBytes(renderDep, renderOpts)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	if err := os.MkdirAll(paths.Rendered, 0o700); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	composePath := filepath.Join(paths.Rendered, "compose.yaml")
	if err := os.WriteFile(composePath, yamlBytes, 0o600); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	if err := materializeBundlePayload(paths.Rendered, vres.Entries); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}
	if err := relabelContainerFiles(filepath.Join(paths.Rendered, "payload")); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 6
		return finish(res)
	}

	labels := map[string]string{
		"iacpack.project": dep.Project.Name, "iacpack.instance": instance,
		"iacpack.slug": slug, "iacpack.run": runID, "iacpack.bundle": bundleSum,
	}

	actions, _ := pres.Model["actions"].([]plan.PlannedAction)
	order, _ := pres.Model["order"].([]string)
	byID := map[string]plan.PlannedAction{}
	for _, a := range actions {
		byID[a.ID] = a
	}

	failedStart := map[string]bool{}
	var failed bool

	exec := func(a plan.PlannedAction, fn func() error) error {
		_ = js.Transition(runID, a.ID, a.Reversible, journal.StatePlanned, nil, "")
		_ = js.Transition(runID, a.ID, a.Reversible, journal.StateStarted, nil, "")
		res.Mutated = true
		if err := fn(); err != nil {
			_ = js.Transition(runID, a.ID, a.Reversible, journal.StateFailed, nil, redactErr(err))
			return err
		}
		_ = js.Transition(runID, a.ID, a.Reversible, journal.StateSucceeded, []string{a.ID}, "")
		return nil
	}

	imageUsers := map[string]string{}
	runtimeUser := func(imageID, explicit string) (string, error) {
		if strings.TrimSpace(explicit) != "" {
			return explicit, nil
		}
		img := dep.Images[imageID]
		ref := img.Ref
		if ref == "" {
			return "", nil
		}
		if u, ok := imageUsers[ref]; ok {
			return u, nil
		}
		type resolver interface {
			ImageUserID(context.Context, string) (string, string, bool, error)
		}
		r, ok := opts.Provider.(resolver)
		if !ok {
			imageUsers[ref] = ""
			return "", nil
		}
		uid, gid, ok, err := r.ImageUserID(ctx, ref)
		if err != nil {
			return "", err
		}
		u := ""
		if ok {
			u = uid + ":" + gid
		}
		imageUsers[ref] = u
		return u, nil
	}

	for _, id := range order {
		a := byID[id]
		if err := ctx.Err(); err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "cancelled"))
			res.ExitHint = 7
			return finish(res)
		}
		switch a.Kind {
		case "verify":
			if err := exec(a, func() error { return nil }); err != nil {
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				return finish(res)
			}
		case "provision.network":
			if err := exec(a, func() error {
				return opts.Provider.EnsureNetwork(ctx, slug, networkCreateName(dep, a.Target), labels)
			}); err != nil {
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				return finish(res)
			}
		case "provision.volume":
			if err := exec(a, func() error {
				return opts.Provider.EnsureVolume(ctx, slug, a.Target, labels)
			}); err != nil {
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				return finish(res)
			}
		case "provision.resource":
			if err := exec(a, func() error { return nil }); err != nil {
				res.ExitHint = 7
				return finish(res)
			}
		case "image.available":
			img := dep.Images[a.Target]
			ref := img.Ref
			if ref == "" && img.Build != nil {
				if !opts.AllowDevBuild {
					res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.DEV_BUILD",
						"development build requires --allow-development-build --yes"))
					res.ExitHint = 6
					return finish(res)
				}
				ref = "build:" + a.Target
			}
			if err := exec(a, func() error { return opts.Provider.ImageAvailable(ctx, ref) }); err != nil {
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				return finish(res)
			}
		case "task.run":
			if a.Rerun == "manual" {
				continue // only via iacpack task run
			}
			task := dep.Tasks[a.Target]
			var inputHash string
			if a.Rerun == "oncePerDeployment" {
				inputHash = migrationInputHash(task, vres.Entries, deployVals)
				if prev, ok := prevInputs[a.Target]; ok && prev == inputHash {
					continue
				}
			}
			if a.CommitBarrier && !opts.Yes {
				// still proceed in non-interactive tests when Yes set; otherwise require Yes for irreversible
			}
			timeout := taskRunTimeout(task.Timeout)
			if err := exec(a, func() error {
				if err := RequireSecretFiles(paths.Secrets, TaskSecretIDs(dep, a.Target)); err != nil {
					return err
				}
				user, err := runtimeUser(task.Image, task.User)
				if err != nil {
					return err
				}
				if err := ownSecretFiles(ctx, opts.Provider, user, paths.Secrets, TaskSecretIDs(dep, a.Target)); err != nil {
					return err
				}
				if err := relabelContainerFiles(paths.Secrets); err != nil {
					return err
				}
				if err := ownWritableMounts(ctx, opts.Provider, slug, paths.Rendered, ir.Service{User: user, Mounts: task.Mounts}, deployVals); err != nil {
					return err
				}
				if err := relabelHostBinds(ir.Service{Mounts: task.Mounts}, deployVals); err != nil {
					return err
				}
				if err := opts.Provider.RunTask(ctx, slug, composePath, a.Target, timeout); err != nil {
					return err
				}
				if inputHash != "" {
					prevInputs[a.Target] = inputHash
					if err := saveTaskInputs(inputPath, prevInputs); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				return finish(res)
			}
		case "service.start":
			var gpuWarn string
			if err := exec(a, func() error {
				if err := RequireSecretFiles(paths.Secrets, ServiceSecretIDs(dep, a.Target)); err != nil {
					return err
				}
				svc := renderDep.Services[a.Target]
				user, err := runtimeUser(svc.Image, svc.User)
				if err != nil {
					return err
				}
				if err := ownSecretFiles(ctx, opts.Provider, user, paths.Secrets, ServiceSecretIDs(dep, a.Target)); err != nil {
					return err
				}
				if err := relabelContainerFiles(paths.Secrets); err != nil {
					return err
				}
				owned := svc
				owned.User = user
				if err := ownWritableMounts(ctx, opts.Provider, slug, paths.Rendered, owned, deployVals); err != nil {
					return err
				}
				if err := relabelHostBinds(owned, deployVals); err != nil {
					return err
				}
				extra, err := secretComposeEnv(paths.Secrets, dep)
				if err != nil {
					return err
				}
				err = opts.Provider.ComposeUp(ctx, slug, composePath, a.Target, extra)
				if err == nil {
					return assertServiceWritableMounts(ctx, opts.Provider, slug, a.Target, renderDep)
				}
				var cdi *provider.CDIUnavailableError
				if errors.As(err, &cdi) && cdi.Device == provider.NVIDIACountAllCDIDevice && serviceHasGPUAll(renderDep, a.Target) {
					if rec, ok := opts.Provider.(interface {
						ComposeUpRecreate(context.Context, string, string, string, []string) error
					}); ok {
						err2 := rec.ComposeUpRecreate(ctx, slug, composePath, a.Target, extra)
						if err2 == nil {
							return assertServiceWritableMounts(ctx, opts.Provider, slug, a.Target, renderDep)
						}
						if !errors.As(err2, &cdi) {
							return err2
						}
						err = err2
					}
				}
				if errors.As(err, &cdi) && cdi.Device == provider.NVIDIACountAllCDIDevice && serviceHasGPUAll(renderDep, a.Target) {
					return err
				}
				return err
			}); err != nil {
				failed = true
				failedStart[a.Target] = true
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, serviceStartDiagnostic(a.Target, err))
				continue
			}
			if gpuWarn != "" {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Warning(codeApplyGPUUnavailable, gpuWarn))
			}
		case "service.ready":
			if failedStart[a.Target] {
				continue
			}
			if err := exec(a, func() error {
				return opts.Provider.WaitReady(ctx, slug, a.Target, 2*time.Minute)
			}); err != nil {
				failed = true
				failedStart[a.Target] = true
				res.ExitHint = 7
				msg := err.Error()
				if a.Target != "" && !strings.Contains(msg, a.Target) {
					msg = fmt.Sprintf("service %q: %s", a.Target, msg)
				}
				if tail := serviceLogTail(ctx, opts.Provider, slug, a.Target, 80); tail != "" {
					msg = msg + "\n" + tail
				}
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", msg))
				continue
			}
			if err := ensureServiceDatabase(ctx, opts.Provider, slug, a.Target, renderDep, deployVals); err != nil {
				failed = true
				res.ExitHint = 7
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error("APPLY.ACTION", err.Error()))
				continue
			}
		case "finalize":
			if err := exec(a, func() error { return nil }); err != nil {
				res.ExitHint = 7
				return finish(res)
			}
		}
	}

	if failed {
		res.ExitHint = 7
		return finish(res)
	}

	owned, _ := opts.Provider.OwnedIDs(ctx, slug)
	doc := state.Document{
		FormatVersion: state.FormatVersion,
		Project:       dep.Project.Name,
		Instance:      instance,
		ProviderSlug:  slug,
		Active: &state.ActiveBundle{
			Digest: bundleSum, SemanticHash: dep.SemanticHash, Path: opts.Bundle,
		},
		LastRun: &state.RunRef{ID: runID, Status: "succeeded"},
	}
	for _, id := range owned {
		doc.Owned = append(doc.Owned, state.OwnedResource{Kind: "compose", ID: id})
	}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	raw = append(raw, '\n')
	tmp := paths.State + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 7
		return finish(res)
	}
	if err := os.Rename(tmp, paths.State); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 7
		return finish(res)
	}
	if err := writeKitHostStamp(paths.Root, kitStamp); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 7
		return finish(res)
	}
	if err := writeRenderHostStamp(paths.Root, hostStamp); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.ExitHint = 7
		return finish(res)
	}

	res.OK = true
	res.Model = map[string]any{
		"project": dep.Project.Name, "instance": instance, "providerSlug": slug,
		"runId": runID, "semanticHash": dep.SemanticHash, "bundleDigest": bundleSum,
		"noop": false, "actions": len(order),
	}
	res.Text = fmt.Sprintf("applied %s@%s instance=%s run=%s\n", dep.Project.Name, dep.Project.Version, instance, runID)
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}

func deployParamValues(dep ir.Deployment, set map[string]string, values map[string]any) map[string]string {
	out := map[string]string{}
	for id, p := range dep.Parameters {
		if p.Phase != "deploy" {
			continue
		}
		if v, ok := set[id]; ok {
			out[id] = v
			continue
		}
		if values != nil {
			if raw, ok := values[id]; ok {
				out[id] = fmt.Sprint(raw)
				continue
			}
		}
		if p.Default != nil {
			out[id] = fmt.Sprint(p.Default)
		}
	}
	return out
}

type writableOwner interface {
	OwnWritable(ctx context.Context, project, user string, binds, volumes []string) error
}

type logTailer interface {
	ServiceLogs(ctx context.Context, project, service string, lines int) (string, error)
}

func ownWritableMounts(ctx context.Context, p provider.ApplyOps, project, rendered string, svc ir.Service, deploy map[string]string) error {
	owner, ok := p.(writableOwner)
	if !ok {
		return nil
	}
	binds, volumes := writableTargets(rendered, svc, deploy)
	if len(binds) == 0 && len(volumes) == 0 {
		return nil
	}
	return owner.OwnWritable(ctx, project, svc.User, binds, volumes)
}

type secretOwner interface {
	OwnSecrets(ctx context.Context, user string, paths []string) error
}

func ownSecretFiles(ctx context.Context, p provider.ApplyOps, user, secretsDir string, ids []string) error {
	owner, ok := p.(secretOwner)
	if !ok || user == "" {
		return nil
	}
	var paths []string
	for _, id := range ids {
		if id == "" {
			continue
		}
		paths = append(paths, filepath.Join(secretsDir, id))
	}
	if len(paths) == 0 {
		return nil
	}
	return owner.OwnSecrets(ctx, user, paths)
}

func serviceLogTail(ctx context.Context, p provider.ApplyOps, project, service string, lines int) string {
	tailer, ok := p.(logTailer)
	if !ok {
		return ""
	}
	text, err := tailer.ServiceLogs(ctx, project, service, lines)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func fileSHA(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func secretComposeEnv(dir string, dep ir.Deployment) ([]string, error) {
	need := map[string]struct{}{}
	note := func(id string) {
		if id != "" {
			need[id] = struct{}{}
		}
	}
	var noteVS func(ir.ValueSource)
	noteVS = func(vs ir.ValueSource) {
		if len(vs.Parts) > 0 {
			for _, p := range vs.Parts {
				noteVS(p)
			}
			return
		}
		note(vs.Secret)
	}
	noteArgv := func(cmd []ir.ArgvElement) {
		for _, el := range cmd {
			note(el.Secret)
			for _, p := range el.Parts {
				note(p.Secret)
			}
		}
	}
	for _, svc := range dep.Services {
		for _, vs := range svc.Environment {
			noteVS(vs)
		}
		noteArgv(svc.Command)
		noteArgv(svc.Entrypoint)
	}
	for _, task := range dep.Tasks {
		noteArgv(task.Command)
	}
	if len(need) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(need))
	for id := range need {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	env := make([]string, 0, len(ids))
	for _, id := range ids {
		b, err := os.ReadFile(filepath.Join(dir, id))
		if err != nil {
			return nil, fmt.Errorf("secret %s is not readable", id)
		}
		val := strings.TrimSuffix(string(b), "\n")
		env = append(env, secrets.ComposeVar(id)+"="+val)
	}
	return env, nil
}

func readKitHostStamp(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "kit-hosts.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeKitHostStamp(root, stamp string) error {
	return writeStampFile(root, "kit-hosts.txt", stamp)
}

func readRenderHostStamp(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "render-host.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeRenderHostStamp(root, stamp string) error {
	return writeStampFile(root, "render-host.txt", stamp)
}

func writeStampFile(root, name, stamp string) error {
	if root == "" {
		return nil
	}
	path := filepath.Join(root, name)
	if stamp == "" {
		_ = os.Remove(path)
		return nil
	}
	return os.WriteFile(path, []byte(stamp+"\n"), 0o600)
}

// hostRenderStamp changes when the host GPU nodes change or when devices-only
// stops hiding them. The bundle hash does not include that, so a same-archive
// re-apply must not noop across this change.
func selinuxEnforcing() bool {
	b, err := os.ReadFile("/sys/fs/selinux/enforce")
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

func selinuxStamp() string {
	if selinuxEnforcing() {
		return "enforcing"
	}
	return "off"
}

func hostRenderStamp() string {
	devs := hostNVIDIADevices()
	body := "devices-only-selinux-v2\n" + selinuxStamp() + "\n" + strings.Join(devs, "\n") + "\n" + compose.VisibleNVIDIADevices(devs)
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
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

func redactErr(err error) string {
	s := err.Error()
	if len(s) > 256 {
		return s[:256]
	}
	return s
}

// taskRunTimeout uses the manifest timeout. The fallback is only for a task
// that does not declare one. A large SQL baseline needs the declared 30m;
// cutting it at 5m kills the transaction and the rollback looks like a hang.
func taskRunTimeout(raw string) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && d > 0 {
		return d
	}
	return 5 * time.Minute
}
