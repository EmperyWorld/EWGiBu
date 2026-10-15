package apply

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/params"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/provider"
	"iacpack/internal/runtime/secrets"
)

const (
	codeImageLocal = "APPLY.IMAGE_LOCAL"
	codeImagePull  = "APPLY.IMAGE_PULL"
	codeHostPath   = "APPLY.HOST_PATH"
)

// imagePresent reports whether ref is already in the engine.
// checked is false when this apply has no engine inspector (unit doubles).
type imagePresent func(ref string) (present bool, checked bool)

type imagePresenter interface {
	ImagePresent(ctx context.Context, ref string) (bool, error)
}

func imagePresentFor(ctx context.Context, p provider.ApplyOps) imagePresent {
	checker, ok := p.(imagePresenter)
	if !ok || checker == nil {
		return nil
	}
	return func(ref string) (bool, bool) {
		present, err := checker.ImagePresent(ctx, ref)
		if err != nil {
			return false, true
		}
		return present, true
	}
}

// preflightDeployment reports what apply will pull and what must be fixed
// before any state or container change. Infos do not fail the run.
func preflightDeployment(dep ir.Deployment, deploy map[string]string, allowDev bool, secretRefs []secrets.Ref, present imagePresent) []diagnostic.Diagnostic {
	var diags []diagnostic.Diagnostic
	diags = append(diags, secrets.Validate(dep.Secrets, secretRefs)...)
	diags = append(diags, missingDeployParams(dep, deploy)...)
	diags = append(diags, missingHostPaths(dep, deploy)...)
	diags = append(diags, imagePreflight(dep, deploy, allowDev, present)...)
	return diags
}

func missingDeployParams(dep ir.Deployment, deploy map[string]string) []diagnostic.Diagnostic {
	resolved := map[string]params.Resolved{}
	for id, v := range deploy {
		resolved[id] = params.Resolved{ID: id, Value: domain.Scalar{Kind: domain.ScalarString, Str: v}}
	}
	_, missing := compose.CanPreview(dep, resolved)
	if len(missing) == 0 {
		return nil
	}
	return []diagnostic.Diagnostic{diagnostic.Error(params.CodeParamMissing,
		"deploy parameters have no value: "+strings.Join(missing, ", ")).
		WithHint("Pass --values or --set. Secret values stay in --secrets-dir, not in the values file")}
}

func missingHostPaths(dep ir.Deployment, deploy map[string]string) []diagnostic.Diagnostic {
	type row struct{ label, path string }
	var rows []row
	add := func(owner string, mounts []ir.Mount) {
		for _, m := range mounts {
			if m.HostLiteral == "" && m.HostParameter == "" {
				continue
			}
			path := m.HostLiteral
			label := owner + ":" + m.Target
			if m.HostParameter != "" {
				path = deploy[m.HostParameter]
				label = m.HostParameter
			}
			if strings.TrimSpace(path) == "" {
				continue
			}
			if !filepath.IsAbs(path) || strings.ContainsAny(path, "\n\r\x00") {
				rows = append(rows, row{label: label, path: path})
				continue
			}
			if _, err := os.Stat(path); err != nil {
				rows = append(rows, row{label: label, path: path})
			}
		}
	}
	sids := sortedServiceIDs(dep)
	for _, id := range sids {
		add(id, dep.Services[id].Mounts)
	}
	tids := sortedTaskIDs(dep)
	for _, id := range tids {
		add(id, dep.Tasks[id].Mounts)
	}
	if len(rows) == 0 {
		return nil
	}
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = r.label + "=" + r.path
	}
	sort.Strings(parts)
	return []diagnostic.Diagnostic{diagnostic.Error(codeHostPath,
		"host path is not an existing absolute path: "+strings.Join(parts, ", ")).
		WithHint("Create the directory on this machine or change the value in values.json")}
}

func imagePreflight(dep ir.Deployment, deploy map[string]string, allowDev bool, present imagePresent) []diagnostic.Diagnostic {
	if present == nil {
		return nil
	}
	refs := usedImageRefs(dep, deploy)
	var pull, local []string
	for _, ref := range refs {
		ok, checked := present(ref)
		if !checked || ok {
			continue
		}
		if imagePullable(ref) {
			pull = append(pull, ref)
			continue
		}
		if strings.HasPrefix(ref, "build:") && allowDev {
			continue
		}
		local = append(local, ref)
	}
	sort.Strings(pull)
	sort.Strings(local)
	var diags []diagnostic.Diagnostic
	if len(pull) > 0 {
		diags = append(diags, diagnostic.Info(codeImagePull,
			"registry images are not local yet:\n"+strings.Join(pull, "\n")).
			WithHint("podman pull "+strings.Join(pull, " ")))
	}
	if len(local) > 0 {
		diags = append(diags, diagnostic.Error(codeImageLocal,
			"not in the engine and pull will not fetch:\n"+strings.Join(local, "\n")).
			WithHint("podman load the image archive, or build these tags from the project repository"))
	}
	return diags
}

func usedImageRefs(dep ir.Deployment, deploy map[string]string) []string {
	ids := map[string]struct{}{}
	for _, svc := range dep.Services {
		if svc.Image != "" {
			ids[svc.Image] = struct{}{}
		}
	}
	for _, task := range dep.Tasks {
		if task.Image != "" {
			ids[task.Image] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var refs []string
	for id := range ids {
		img := dep.Images[id]
		ref := img.Ref
		if ref == "" && img.RefParameter != "" {
			ref = deploy[img.RefParameter]
		}
		if ref == "" && img.Build != nil {
			ref = "build:" + id
		}
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func missingPullRefs(dep ir.Deployment, deploy map[string]string, present imagePresent) []string {
	if present == nil {
		return nil
	}
	var pull []string
	for _, ref := range usedImageRefs(dep, deploy) {
		ok, checked := present(ref)
		if !checked || ok || !imagePullable(ref) {
			continue
		}
		pull = append(pull, ref)
	}
	sort.Strings(pull)
	return pull
}

func imagePullable(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "build:") {
		return false
	}
	low := strings.ToLower(ref)
	if strings.HasPrefix(low, "localhost/") || strings.HasPrefix(low, "localhost:") || strings.HasPrefix(low, "127.0.0.1/") {
		return false
	}
	return strings.Contains(ref, "/")
}

func networkCreateName(dep ir.Deployment, id string) string {
	n, ok := dep.Networks[id]
	if ok && n.External != nil && n.External.Name != "" {
		return n.External.Name
	}
	return id
}

func sortedServiceIDs(dep ir.Deployment) []string {
	ids := make([]string, 0, len(dep.Services))
	for id := range dep.Services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
