// Package validate implements the pure validate use-case.
package validate

import (
	"bytes"
	"path/filepath"
	"strings"

	"iacpack/internal/app/frontend"
	"iacpack/internal/diagnostic"
	domain "iacpack/internal/domain/manifest"
	"iacpack/internal/inputs"
	"iacpack/internal/lockfile"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/oci"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

// FileReader reads a single file by path. Adapters inject filesystem access.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// Options configures validation depth and explicit parameters.
type Options struct {
	SyntaxOnly   bool
	Locked       bool
	LockPath     string
	Policy       policy.CompilePolicy
	Set          map[string]string
	Values       map[string]any
	Workspace    inputs.FS // required when not SyntaxOnly and inputs exist; may be nil if no inputs
	UpdateImages bool
	Resolver     oci.Resolver
	OutputRel    string // optional workspace-relative output path to reject
}

// Result is the outcome of validating one manifest file.
type Result struct {
	Project      domain.Project
	Diagnostics  []diagnostic.Diagnostic
	Canonical    any
	SourceBytes  []byte
	OK           bool
	Policy       policy.CompilePolicy
	BuildParams  map[string]params.Resolved
	Inputs       []inputs.FileRecord
	LockDocument lockfile.Document
	SyntaxOnly   bool
}

// Run loads and validates a manifest with the default compile policy (full).
func Run(fs FileReader, path string) Result {
	return RunWithOptions(fs, path, Options{Policy: policy.MustDefault()})
}

// RunWithPolicy loads and validates a manifest against an explicit compile policy.
func RunWithPolicy(fs FileReader, path string, pol policy.CompilePolicy) Result {
	return RunWithOptions(fs, path, Options{Policy: pol})
}

// RunWithOptions runs validation with the given options.
func RunWithOptions(fs FileReader, path string, opts Options) Result {
	if opts.Policy.APIVersion == "" {
		opts.Policy = policy.MustDefault()
	}
	data, err := fs.ReadFile(path)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read manifest: "+err.Error()).
			WithSource(path, "", 0, 0)
		return Result{Diagnostics: []diagnostic.Diagnostic{d}, OK: false, Policy: opts.Policy, SyntaxOnly: opts.SyntaxOnly}
	}
	snapshot := append([]byte(nil), data...)

	doc := loader.Parse(path, data)
	if !bytes.Equal(snapshot, data) {
		d := diagnostic.Error(diagnostic.CodeInternal, "loader mutated input bytes").
			WithSource(path, "", 0, 0)
		return Result{Diagnostics: []diagnostic.Diagnostic{d}, SourceBytes: snapshot, OK: false, Policy: opts.Policy, SyntaxOnly: opts.SyntaxOnly}
	}
	project, diags := v1alpha1.Decode(doc)
	res := Result{
		Project:     project,
		Diagnostics: diags,
		SourceBytes: snapshot,
		Policy:      opts.Policy,
		SyntaxOnly:  opts.SyntaxOnly,
	}
	if doc.Root != nil {
		res.Canonical = doc.Root.AsCanonical()
	}
	if diagnostic.HasError(diags) {
		res.OK = false
		diagnostic.SortDeterministic(res.Diagnostics)
		return res
	}

	if opts.SyntaxOnly {
		// Parser/schema/decode only — insufficient as a release gate.
		res.Diagnostics = append(res.Diagnostics, diagnostic.Warning(diagnostic.CodeValidateSyntaxOnly,
			"syntax-only validation skipped input, parameter, and policy checks; not a release gate"))
		diagnostic.SortDeterministic(res.Diagnostics)
		res.OK = !diagnostic.HasError(res.Diagnostics)
		return res
	}

	res.Diagnostics = append(res.Diagnostics, policy.Evaluate(opts.Policy, project)...)

	buildParams, pdiags := params.ResolveBuild(project, params.Options{Set: opts.Set, Values: opts.Values})
	res.Diagnostics = append(res.Diagnostics, pdiags...)
	res.BuildParams = buildParams

	needsInputs := len(project.Spec.Resources) > 0 || hasBuildImage(project) || len(project.Spec.Builds) > 0 || len(project.Spec.Databases) > 0
	if len(project.Spec.Builds) > 0 {
		res.Diagnostics = append(res.Diagnostics, frontend.Check(opts.Workspace, project.Spec.Builds)...)
	}
	if needsInputs {
		if opts.Workspace == nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(inputs.CodeInputIO,
				"workspace filesystem is required to validate declared inputs"))
		} else {
			recs, idiags := inputs.ResolveProject(opts.Workspace, project, inputs.ResolveOptions{OutputRel: opts.OutputRel})
			res.Diagnostics = append(res.Diagnostics, idiags...)
			res.Inputs = recs
		}
	}

	if !diagnostic.HasError(res.Diagnostics) {
		docLock, ldiags := lockfile.Build(lockfile.BuildOptions{
			ManifestBytes: snapshot,
			Project:       project,
			Policy:        opts.Policy,
			BuildParams:   buildParams,
			Inputs:        res.Inputs,
			UpdateImages:  opts.UpdateImages,
			Resolver:      opts.Resolver,
		})
		res.Diagnostics = append(res.Diagnostics, ldiags...)
		res.LockDocument = docLock
	}

	if opts.Locked && !diagnostic.HasError(res.Diagnostics) {
		lockPath := opts.LockPath
		if lockPath == "" {
			lockPath = defaultLockPath(path)
		}
		raw, err := fs.ReadFile(lockPath)
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse,
				"failed to read lockfile: "+err.Error()).WithSource(lockPath, "", 0, 0))
		} else {
			locked, err := lockfile.Parse(raw)
			if err != nil {
				res.Diagnostics = append(res.Diagnostics, diagnostic.Error(lockfile.CodeLockParse, err.Error()).
					WithSource(lockPath, "", 0, 0))
			} else {
				res.Diagnostics = append(res.Diagnostics, lockfile.CompareDrift(locked, res.LockDocument)...)
			}
		}
	}

	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = !diagnostic.HasError(res.Diagnostics)
	return res
}

func hasBuildImage(p domain.Project) bool {
	for _, img := range p.Spec.Images {
		if img.Build != nil {
			return true
		}
	}
	return false
}

func defaultLockPath(manifestPath string) string {
	dir := filepath.Dir(manifestPath)
	return filepath.Join(dir, "iacpack.lock.json")
}

// WorkspaceRootFromManifest returns the directory containing the manifest.
func WorkspaceRootFromManifest(manifestPath string) string {
	return filepath.Dir(manifestPath)
}

// RelOutputPath converts an absolute or relative output path into a workspace-relative path if possible.
func RelOutputPath(workspaceRoot, outputPath string) string {
	if outputPath == "" {
		return ""
	}
	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return ""
	}
	absWS, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(absWS, absOut)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}
