// Package migrate implements explicit manifest schema migration plumbing.
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

// FileReader reads source manifests.
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// Writer creates a new output file exclusively.
type Writer interface {
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
}

// Options configures migration.
type Options struct {
	File   string
	To     string
	Output string
}

// Result is the migrate outcome.
type Result struct {
	OK          bool
	Diagnostics []diagnostic.Diagnostic
	Model       map[string]any
	Text        string
}

const apiV1Alpha1 = "iacpack.io/v1alpha1"

// Run migrates a manifest to an explicit target apiVersion into a new output path.
func Run(fs FileReader, w Writer, opts Options) Result {
	var res Result
	if opts.File == "" || opts.To == "" || opts.Output == "" {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"manifest migrate requires --file, --to, and --output")}
		return finish(res)
	}
	raw, err := fs.ReadFile(opts.File)
	if err != nil {
		res.Diagnostics = []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
		return finish(res)
	}
	doc := loader.Parse(opts.File, raw)
	if diagnostic.HasError(doc.Diags) {
		res.Diagnostics = doc.Diags
		return finish(res)
	}
	proj, pdiags := v1alpha1.Decode(doc)
	res.Diagnostics = append(res.Diagnostics, pdiags...)
	if diagnostic.HasError(res.Diagnostics) {
		return finish(res)
	}
	srcAPI := strings.TrimSpace(proj.APIVersion)
	target := strings.TrimSpace(opts.To)
	if srcAPI == "" {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeManifestUnsupportedAPI, "missing apiVersion"))
		return finish(res)
	}
	if srcAPI != apiV1Alpha1 {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeManifestUnsupportedAPI,
			fmt.Sprintf("unsupported source apiVersion %q", srcAPI)))
		return finish(res)
	}
	if target != apiV1Alpha1 {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeManifestUnsupportedAPI,
			fmt.Sprintf("unsupported target apiVersion %q; only %s is available", target, apiV1Alpha1)).
			WithHint("Do not rewrite in place; provide a supported --to value"))
		return finish(res)
	}
	// Same-version: emit canonical semantic JSON (never silent in-place rewrite).
	canon, err := v1alpha1.CanonicalSemantic(proj)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
		return finish(res)
	}
	out := append(canon, '\n')
	if filepath.Clean(opts.Output) == filepath.Clean(opts.File) {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeCLIInvalidUsage,
			"refusing in-place migrate; choose a distinct --output path"))
		return finish(res)
	}
	if err := w.WriteFileAtomic(opts.Output, out, 0o644); err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		return finish(res)
	}
	res.OK = true
	res.Model = map[string]any{
		"from":   srcAPI,
		"to":     target,
		"output": opts.Output,
		"mode":   "canonical-rewrite",
		"bytes":  len(out),
	}
	res.Text = fmt.Sprintf("migrated %s -> %s (%s)\nwrote %s\n", srcAPI, target, "same-version canonical", opts.Output)
	return finish(res)
}

func finish(res Result) Result {
	diagnostic.SortDeterministic(res.Diagnostics)
	res.OK = res.OK && !diagnostic.HasError(res.Diagnostics)
	return res
}
