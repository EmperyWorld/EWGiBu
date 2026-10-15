// Package render materializes verified deployment documents for operator use.
package render

import (
	"fmt"
	"os"

	"iacpack/internal/app/verify"
	"iacpack/internal/bundle"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/lockfile"
	"iacpack/internal/params"
	"iacpack/internal/render/compose"
)

// Writer creates a new exclusive output file.
type Writer interface {
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
}

// Options configures compose rendering from a verified bundle.
type Options struct {
	Bundle string
	Set    map[string]string
	Values map[string]any
	Output string // empty => stdout bytes in Result.Bytes / Text
}

// Result is the render outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	Bytes       []byte
}

// Compose verifies a bundle and renders Compose from deployment.json + deploy values.
func Compose(w Writer, opts Options) Result {
	var res Result
	if opts.Bundle == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "render compose requires a bundle path")}
		return finish(res)
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
	lockRaw, err := vres.ReadFile("iacpack.lock.json")
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(bundle.CodeBundleMissing, "iacpack.lock.json missing from bundle"))
		return finish(res)
	}
	locked, err := lockfile.Parse(lockRaw)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse, err.Error()))
		return finish(res)
	}

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
		resolved[id] = params.Resolved{
			ID: id, Value: domain.Scalar{Kind: domain.ScalarString, Str: v}, Source: params.SourceCLISet,
		}
	}
	can, missing := compose.CanPreview(dep, resolved)
	if !can {
		for _, id := range missing {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(params.CodeParamMissing,
				fmt.Sprintf("deploy parameter %q required for render", id)).
				WithHint("Pass --set "+id+"=<value> or --values"))
		}
		return finish(res)
	}
	cf, err := compose.Render(dep, compose.RenderOptions{DeployValues: deployVals, Lock: locked})
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		return finish(res)
	}
	raw, err := compose.MarshalYAML(cf)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error("RENDER.COMPOSE", err.Error()))
		return finish(res)
	}
	res.Bytes = raw
	if opts.Output != "" {
		if w == nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "writer required for --output"))
			return finish(res)
		}
		if err := w.WriteFileAtomic(opts.Output, raw, 0o644); err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
			return finish(res)
		}
	}
	res.OK = true
	res.Text = string(raw)
	res.Model = map[string]any{
		"bundle":       opts.Bundle,
		"format":       "compose",
		"bytes":        len(raw),
		"output":       opts.Output,
		"project":      dep.Project.Name,
		"semanticHash": dep.SemanticHash,
	}
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
