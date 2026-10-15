// Package graph implements inspect-graph compilation (pure; no Docker/network/writes).
package graph

import (
	"iacpack/internal/app/validate"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/ir"
	"iacpack/internal/domain/plan"
	"iacpack/internal/policy"
)

// Result is the outcome of compiling and inspecting a graph.
type Result struct {
	Deployment  ir.Deployment
	Graph       plan.Graph
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	Mermaid     string
	OK          bool
}

// Run validates (full) then compiles IR + action graph.
func Run(fs validate.FileReader, path string, opts validate.Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	vres := validate.RunWithOptions(fs, path, opts)
	res := Result{Diagnostics: append([]diagnostic.Diagnostic{}, vres.Diagnostics...)}
	if !vres.OK {
		res.OK = false
		return res
	}
	dep, err := compile.ToIR(vres.Project)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, "IR compile: "+err.Error()))
		res.OK = false
		return res
	}
	g, gdiags := compile.BuildGraph(dep)
	res.Diagnostics = append(res.Diagnostics, gdiags...)
	res.Deployment = dep
	res.Graph = g
	if diagnostic.HasError(res.Diagnostics) {
		res.OK = false
		diagnostic.SortDeterministic(res.Diagnostics)
		return res
	}
	res.Model = compile.GraphInspectModel(dep, g)
	res.Text = compile.FormatGraphText(dep, g)
	res.Mermaid = compile.FormatGraphMermaid(g)
	res.OK = true
	diagnostic.SortDeterministic(res.Diagnostics)
	return res
}
