package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/build"
	"iacpack/internal/diagnostic"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

func runBuild(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	lockPath := ""
	policyPath := ""
	valuesPath := ""
	outPath := ""
	format := "directory"
	embedImages := false
	imageCache := ""
	var setPairs []string
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack build --file <path> --output <path> [--lock <path>] [--format directory|bundle] [--embed-images --image-cache <dir>] [--policy <path>] [--values <path>] [--set k=v] [--output text|json]")
			fmt.Fprintln(stdout, "  Compiles a locked project into a new directory or .iacbundle artifact.")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--lock":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--lock requires a value")}, ExitUsage)
			}
			lockPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--lock="):
			lockPath = strings.TrimPrefix(a, "--lock=")
		case a == "--policy":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--policy requires a value")}, ExitUsage)
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--values":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--values requires a value")}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--set":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--set requires name=value")}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--format":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--format requires a value")}, ExitUsage)
			}
			format = args[i+1]
			i++
		case strings.HasPrefix(a, "--format="):
			format = strings.TrimPrefix(a, "--format=")
		case a == "--embed-images":
			embedImages = true
		case a == "--image-cache":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--image-cache requires a value")}, ExitUsage)
			}
			imageCache = args[i+1]
			i++
		case strings.HasPrefix(a, "--image-cache="):
			imageCache = strings.TrimPrefix(a, "--image-cache=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			val := args[i+1]
			i++
			if strings.EqualFold(val, "json") || strings.EqualFold(val, "text") {
				outputMode = strings.ToLower(val)
			} else {
				outPath = val
			}
		case strings.HasPrefix(a, "--output="):
			val := strings.TrimPrefix(a, "--output=")
			if strings.EqualFold(val, "json") || strings.EqualFold(val, "text") {
				outputMode = strings.ToLower(val)
			} else {
				outPath = val
			}
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 {
		return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitBuild(stdout, stderr, "text", false, nil, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported machine --output %q", outputMode))}, ExitUsage)
	}
	if filePath == "" {
		return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("build requires --file <path>")}, ExitUsage)
	}
	if outPath == "" {
		return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{usageDiag("build requires --output <dir>")}, ExitUsage)
	}

	pol := policy.MustDefault()
	if policyPath != "" {
		pdata, err := fs.ReadFile(policyPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read policy: "+err.Error())
			return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{d}, ExitIO)
		}
		parsed, err := policy.Parse(pdata)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, "invalid compile policy: "+err.Error())
			return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{d}, ExitInvalid)
		}
		pol = parsed
	}
	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitBuild(stdout, stderr, outputMode, false, nil, setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error())
			return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitBuild(stdout, stderr, outputMode, false, nil, vdiags, ExitInvalid)
		}
	}

	wsRoot, err := workspace.RootForManifest(filePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{d}, ExitIO)
	}
	ws, err := workspace.New(wsRoot)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitBuild(stdout, stderr, outputMode, false, nil, []diagnostic.Diagnostic{d}, ExitIO)
	}

	writer := build.OSWriter{}
	res := build.RunWithBundle(fs, writer, writer, filePath, build.Options{
		Policy:      pol,
		Set:         setMap,
		Values:      values,
		Workspace:   ws,
		LockPath:    lockPath,
		Output:      outPath,
		Format:      format,
		EmbedImages: embedImages,
		ImageCache:  imageCache,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
		// filesystem failures should be ExitIO
		for _, d := range res.Diagnostics {
			if d.Code == diagnostic.CodeInputIOError {
				code = ExitIO
				break
			}
		}
	}
	result := map[string]any{
		"output":  res.Output,
		"format":  format,
		"preview": res.Preview,
		"files":   len(res.TreePaths),
	}
	if format == "bundle" && res.BundleSHA256 != "" {
		result["sha256"] = res.BundleSHA256
	}
	return emitBuild(stdout, stderr, outputMode, res.OK, result, res.Diagnostics, code)
}

func emitBuild(stdout, stderr io.Writer, mode string, ok bool, result any, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if ok {
			if m, okm := result.(map[string]any); okm {
				if m["format"] == "bundle" {
					fmt.Fprintf(stderr, "wrote bundle %v (sha256=%v, files=%v, preview=%v)\n", m["output"], m["sha256"], m["files"], m["preview"])
				} else {
					fmt.Fprintf(stderr, "wrote directory build to %v (%v files, preview=%v)\n", m["output"], m["files"], m["preview"])
				}
			}
		}
		return code
	}
	var r any
	if ok {
		r = result
	}
	if err := WriteJSON(stdout, "build", ok, r, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
