package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/adapters/workspace"
	applock "iacpack/internal/app/lock"
	"iacpack/internal/diagnostic"
	"iacpack/internal/oci"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

func runLock(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	policyPath := ""
	valuesPath := ""
	lockPath := ""
	updateImages := false
	var setPairs []string
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack lock --file <path> [--lock <path>] [--policy <path>] [--values <path>] [--set k=v] [--update-images] [--output text|json]")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, "", nil, []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--lock":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, "", nil, []diagnostic.Diagnostic{usageDiag("--lock requires a value")}, ExitUsage)
			}
			lockPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--lock="):
			lockPath = strings.TrimPrefix(a, "--lock=")
		case a == "--policy":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{usageDiag("--policy requires a value")}, ExitUsage)
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--values":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{usageDiag("--values requires a value")}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--set":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{usageDiag("--set requires name=value")}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--update-images":
			updateImages = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitLock(stdout, stderr, outputMode, false, "", nil, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.TrimPrefix(a, "--output=")
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 {
		return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitLock(stdout, stderr, "text", false, filePath, nil, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if filePath == "" {
		return emitLock(stdout, stderr, outputMode, false, "", nil, []diagnostic.Diagnostic{usageDiag("lock requires --file <path>")}, ExitUsage)
	}

	pol := policy.MustDefault()
	if policyPath != "" {
		pdata, err := fs.ReadFile(policyPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read policy: "+err.Error()).WithSource(policyPath, "", 0, 0)
			return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{d}, ExitIO)
		}
		parsed, err := policy.Parse(pdata)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, "invalid compile policy: "+err.Error()).WithSource(policyPath, "", 0, 0)
			return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{d}, ExitInvalid)
		}
		pol = parsed
	}
	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitLock(stdout, stderr, outputMode, false, filePath, nil, setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error()).WithSource(valuesPath, "", 0, 0)
			return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitLock(stdout, stderr, outputMode, false, filePath, nil, vdiags, ExitInvalid)
		}
	}

	wsRoot, err := workspace.RootForManifest(filePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{d}, ExitIO)
	}
	ws, err := workspace.New(wsRoot)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitLock(stdout, stderr, outputMode, false, filePath, nil, []diagnostic.Diagnostic{d}, ExitIO)
	}

	var resolver oci.Resolver = oci.DeniedResolver{}
	if updateImages {
		// Real registry HTTP is not wired in P03 product path without credentials;
		// inject MapResolver only in tests. Production --update-images without a
		// configured resolver fails closed via DeniedResolver unless replaced later.
		resolver = oci.DeniedResolver{}
	}

	res := applock.Run(fs, filePath, applock.Options{
		Policy:       pol,
		Set:          setMap,
		Values:       values,
		Workspace:    ws,
		LockPath:     lockPath,
		UpdateImages: updateImages,
		Resolver:     resolver,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitLock(stdout, stderr, outputMode, res.OK, res.LockPath, map[string]any{
		"lock":   res.LockPath,
		"inputs": len(res.Inputs),
	}, res.Diagnostics, code)
}

func emitLock(stdout, stderr io.Writer, mode string, ok bool, lockPath string, result any, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if ok {
			fmt.Fprintf(stderr, "wrote %s\n", lockPath)
		}
		return code
	}
	var r any
	if ok {
		r = result
	}
	if err := WriteJSON(stdout, "lock", ok, r, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
