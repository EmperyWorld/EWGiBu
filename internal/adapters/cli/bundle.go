package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/build"
	"iacpack/internal/app/portable"
	"iacpack/internal/diagnostic"
	"iacpack/internal/policy"
)

func runBundle(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	workspace := ""
	outputDir := ""
	embedLocal := false
	policyPath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack bundle --workspace <dir> --output-dir <dir> [--policy <path>] [--embed-local-images] [--output text|json]")
			fmt.Fprintln(stdout, "  Writes a portable kit: lock, values without secrets, required secret names,")
			fmt.Fprintln(stdout, "  images marked pull / local-only / load, host-paths.json as a template,")
			fmt.Fprintln(stdout, "  and project.iacbundle. Empty host paths and unembedded local images are warnings.")
			fmt.Fprintln(stdout, "  plan and apply of the kit are the gate, then they use that archive.")
			fmt.Fprintln(stdout, "  --embed-local-images saves each local-only image as images/*.tar and marks load.")
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitBundle(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspace = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspace = strings.TrimPrefix(a, "--workspace=")
		case a == "--output-dir":
			if i+1 >= len(args) {
				return emitBundle(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output-dir requires a value")}, ExitUsage)
			}
			outputDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--output-dir="):
			outputDir = strings.TrimPrefix(a, "--output-dir=")
		case a == "--embed-local-images":
			embedLocal = true
		case a == "--policy":
			if i+1 >= len(args) {
				return emitBundle(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--policy requires a value")}, ExitUsage)
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitBundle(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.TrimPrefix(a, "--output=")
		default:
			return emitBundle(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitBundle(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	res := portable.Run(portable.Options{Workspace: workspace, OutputDir: outputDir, EmbedLocalImages: embedLocal})
	if res.Model != nil {
		res = attachProjectBundle(fs, workspace, policyPath, outputDir, res)
	}
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitBundle(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func attachProjectBundle(fs FileSystem, workspacePath, policyPath, outDir string, res portable.Result) portable.Result {
	manifest := filepath.Join(workspacePath, "iacpack.yaml")
	if _, err := fs.ReadFile(manifest); err != nil {
		return res
	}
	pol := policy.MustDefault()
	if policyPath != "" {
		raw, err := fs.ReadFile(policyPath)
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, "policy: "+err.Error()))
			res.OK = false
			return res
		}
		parsed, err := policy.Parse(raw)
		if err != nil {
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodePolicyUnsupportedField, err.Error()))
			res.OK = false
			return res
		}
		pol = parsed
	}
	wsRoot, err := workspace.RootForManifest(manifest)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.OK = false
		return res
	}
	ws, err := workspace.New(wsRoot)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
		res.OK = false
		return res
	}
	out := filepath.Join(outDir, "project.iacbundle")
	writer := build.OSWriter{}
	bres := build.RunWithBundle(fs, writer, writer, manifest, build.Options{
		Policy: pol, Workspace: ws, Output: out, Format: "bundle",
	})
	res.Diagnostics = append(res.Diagnostics, bres.Diagnostics...)
	if !bres.OK {
		res.OK = false
		return res
	}
	if res.Model == nil {
		res.Model = map[string]any{}
	}
	res.Model["bundle"] = out
	res.Text += "bundle: " + out + "\n"
	return res
}

func emitBundle(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if text != "" {
			_, _ = io.WriteString(stdout, text)
		}
		return code
	}
	var result any
	if model != nil {
		result = model
	}
	if err := WriteJSON(stdout, "bundle", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
