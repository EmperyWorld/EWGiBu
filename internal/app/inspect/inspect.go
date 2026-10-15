// Package inspect implements inspect-manifest and inspect-inputs use-cases.
package inspect

import (
	"fmt"
	"sort"
	"strings"

	"iacpack/internal/app/validate"
	"iacpack/internal/diagnostic"
	"iacpack/internal/discover"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/inputs"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/oci"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

// Result is the outcome of inspecting one manifest.
type Result struct {
	Project     domain.Project
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	OK          bool
}

// Run validates then produces stable public inspect output.
func Run(fs validate.FileReader, path string) Result {
	return RunWithOptions(fs, path, validate.Options{Policy: policy.MustDefault()})
}

// RunWithOptions validates with options then produces inspect manifest output.
func RunWithOptions(fs validate.FileReader, path string, opts validate.Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	vres := validate.RunWithOptions(fs, path, opts)
	if !vres.OK {
		return Result{
			Project:     vres.Project,
			Diagnostics: vres.Diagnostics,
			OK:          false,
		}
	}
	model, err := v1alpha1.PublicInspectModel(vres.Project)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInternal, "failed to build inspect model: "+err.Error())
		return Result{Diagnostics: []diagnostic.Diagnostic{d}, OK: false}
	}
	text, err := v1alpha1.FormatInspectText(vres.Project)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInternal, "failed to format inspect text: "+err.Error())
		return Result{Diagnostics: []diagnostic.Diagnostic{d}, OK: false}
	}
	return Result{
		Project:     vres.Project,
		Diagnostics: vres.Diagnostics,
		Model:       model,
		Text:        text,
		OK:          true,
	}
}

// InputsResult is inspect inputs output.
type InputsResult struct {
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	OK          bool
	Inputs      []inputs.FileRecord
	BuildParams map[string]params.Resolved
}

// RunInputs resolves declared inputs without writing the workspace.
func RunInputs(fs validate.FileReader, path string, opts validate.Options) InputsResult {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	opts.UpdateImages = false
	if opts.Resolver == nil {
		opts.Resolver = oci.DeniedResolver{}
	}
	vres := validate.RunWithOptions(fs, path, opts)
	res := InputsResult{
		Diagnostics: vres.Diagnostics,
		Inputs:      vres.Inputs,
		BuildParams: vres.BuildParams,
	}
	if !vres.OK {
		res.OK = false
		return res
	}
	files := make([]any, 0, len(vres.Inputs))
	for _, r := range vres.Inputs {
		files = append(files, map[string]any{
			"path":   r.Path,
			"size":   r.Size,
			"mode":   r.Mode,
			"sha256": r.SHA256,
			"source": r.Source,
		})
	}
	imgs := map[string]any{}
	ids := make([]string, 0, len(vres.LockDocument.Images))
	for id := range vres.LockDocument.Images {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		im := vres.LockDocument.Images[id]
		m := map[string]any{"ref": im.Ref}
		if im.Digest != "" {
			m["digest"] = im.Digest
		}
		if im.Build {
			m["build"] = true
		}
		imgs[id] = m
	}
	res.Model = map[string]any{
		"inputs":          files,
		"buildParameters": params.PublicMap(vres.BuildParams),
		"images":          imgs,
		"policy": map[string]any{
			"name":   vres.LockDocument.Policy.Name,
			"sha256": vres.LockDocument.Policy.SHA256,
		},
	}
	res.Text = formatInputsText(vres.Inputs, vres.BuildParams)
	res.OK = true
	return res
}

func formatInputsText(recs []inputs.FileRecord, bp map[string]params.Resolved) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("inputs: %d\n", len(recs)))
	for _, r := range recs {
		b.WriteString(fmt.Sprintf("  %s  mode=%s size=%d sha256=%s source=%s\n", r.Path, r.Mode, r.Size, r.SHA256, r.Source))
	}
	keys := make([]string, 0, len(bp))
	for k := range bp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString(fmt.Sprintf("buildParameters: %d\n", len(keys)))
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("  %s (%s)\n", k, bp[k].Source))
	}
	return b.String()
}

// FormatDiscover builds text/json model from suggestions.
func FormatDiscover(sugs []discover.Suggestion) (text string, model map[string]any) {
	items := make([]any, 0, len(sugs))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("discover: %d suggestion(s)\n", len(sugs)))
	b.WriteString("note: suggestions are reviewable only; they do not change build inputs\n")
	for _, s := range sugs {
		items = append(items, map[string]any{"path": s.Path, "kind": s.Kind})
		b.WriteString(fmt.Sprintf("  %s (%s)\n", s.Path, s.Kind))
	}
	return b.String(), map[string]any{"suggestions": items}
}
