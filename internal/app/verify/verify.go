// Package verify implements hostile-bundle verification (no extraction to disk).
package verify

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"iacpack/internal/bundle"
	"iacpack/internal/diagnostic"
)

// Result is the verify command outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
	Bundle      bundle.Result
}

// Run verifies a bundle file path.
func Run(path string) Result {
	f, err := os.Open(path)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open bundle: "+err.Error()).
			WithSource(path, "", 0, 0)
		return Result{Diagnostics: []diagnostic.Diagnostic{d}}
	}
	defer f.Close()
	return RunReader(f, path)
}

// RunReader verifies an archive stream.
func RunReader(r io.Reader, source string) Result {
	vres, diags := bundle.Verify(r, bundle.VerifyOptions{
		Quotas:       bundle.DefaultQuotas(),
		KeepContents: false,
	})
	if len(diags) > 0 {
		diagnostic.SortDeterministic(diags)
		return Result{Diagnostics: diags, Bundle: vres}
	}
	model := map[string]any{
		"path":           source,
		"formatVersion":  vres.Meta.FormatVersion,
		"project":        map[string]any{"name": vres.Meta.Project.Name, "version": vres.Meta.Project.Version, "mode": vres.Meta.Project.Mode},
		"semanticHash":   vres.Meta.Project.SemanticHash,
		"entries":        len(vres.Meta.Entries),
		"compressedSize": vres.CompressedSize,
		"expandedSize":   vres.ExpandedSize,
		"capabilities": map[string]any{
			"embedImages":    vres.Meta.Capabilities.EmbedImages,
			"includeRuntime": vres.Meta.Capabilities.IncludeRuntime,
			"hasPreview":     vres.Meta.Capabilities.HasPreview,
		},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ok: true\n")
	fmt.Fprintf(&b, "formatVersion: %s\n", vres.Meta.FormatVersion)
	fmt.Fprintf(&b, "project: %s@%s\n", vres.Meta.Project.Name, vres.Meta.Project.Version)
	fmt.Fprintf(&b, "semanticHash: %s\n", vres.Meta.Project.SemanticHash)
	fmt.Fprintf(&b, "entries: %d\n", len(vres.Meta.Entries))
	fmt.Fprintf(&b, "compressedSize: %d\n", vres.CompressedSize)
	fmt.Fprintf(&b, "expandedSize: %d\n", vres.ExpandedSize)
	return Result{OK: true, Model: model, Text: b.String(), Bundle: vres}
}

// Inspect summarizes a verified bundle without retaining payload bytes.
func Inspect(path string) Result {
	f, err := os.Open(path)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open bundle: "+err.Error()).
			WithSource(path, "", 0, 0)
		return Result{Diagnostics: []diagnostic.Diagnostic{d}}
	}
	defer f.Close()
	vres, diags := bundle.Verify(f, bundle.VerifyOptions{Quotas: bundle.DefaultQuotas(), KeepContents: false})
	if len(diags) > 0 {
		diagnostic.SortDeterministic(diags)
		return Result{Diagnostics: diags}
	}
	entryList := make([]map[string]any, 0, len(vres.Meta.Entries))
	for _, e := range vres.Meta.Entries {
		entryList = append(entryList, map[string]any{
			"path": e.Path, "size": e.Size, "mode": e.Mode, "sha256": e.SHA256,
		})
	}
	model := map[string]any{
		"path":          path,
		"formatVersion": vres.Meta.FormatVersion,
		"project": map[string]any{
			"name": vres.Meta.Project.Name, "version": vres.Meta.Project.Version,
			"mode": vres.Meta.Project.Mode, "semanticHash": vres.Meta.Project.SemanticHash,
		},
		"compiler":   map[string]any{"product": vres.Meta.Compiler.Product, "version": vres.Meta.Compiler.Version},
		"provider":   map[string]any{"profile": vres.Meta.Provider.Profile, "version": vres.Meta.Provider.Version},
		"runtime":    map[string]any{"composePluginMin": vres.Meta.Runtime.ComposePluginMin},
		"entries":    entryList,
		"hasPreview": vres.Meta.Capabilities.HasPreview,
	}
	var b strings.Builder
	fmt.Fprintf(&b, "bundle: %s\n", path)
	fmt.Fprintf(&b, "project: %s@%s\n", vres.Meta.Project.Name, vres.Meta.Project.Version)
	fmt.Fprintf(&b, "semanticHash: %s\n", vres.Meta.Project.SemanticHash)
	fmt.Fprintf(&b, "entries:\n")
	for _, e := range vres.Meta.Entries {
		fmt.Fprintf(&b, "  %s  %d  %s\n", e.Path, e.Size, e.SHA256[:12])
	}
	return Result{OK: true, Model: model, Text: b.String(), Bundle: vres}
}

// ReadVerifiedFile verifies a bundle and returns one retained entry (full verify with contents).
func ReadVerifiedFile(path, entry string) ([]byte, bundle.Result, []diagnostic.Diagnostic) {
	raw, err := os.ReadFile(path)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read bundle: "+err.Error())
		return nil, bundle.Result{}, []diagnostic.Diagnostic{d}
	}
	vres, diags := bundle.Verify(bytes.NewReader(raw), bundle.VerifyOptions{
		Quotas:       bundle.DefaultQuotas(),
		KeepContents: true,
	})
	if len(diags) > 0 {
		return nil, bundle.Result{}, diags
	}
	data, err := vres.ReadFile(entry)
	if err != nil {
		return nil, vres, []diagnostic.Diagnostic{diagnostic.Error(bundle.CodeBundleMissing, err.Error())}
	}
	return data, vres, nil
}
