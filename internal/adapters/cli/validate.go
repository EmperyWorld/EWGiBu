package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/validate"
	"iacpack/internal/diagnostic"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

func runValidate(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	policyPath := ""
	valuesPath := ""
	lockPath := ""
	syntaxOnly := false
	locked := false
	var setPairs []string
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack validate --file <path> [--policy <path>] [--values <path>] [--set k=v] [--locked] [--lock <path>] [--syntax-only] [--output text|json]")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				d := usageDiag("--file requires a value")
				return emitValidate(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--policy":
			if i+1 >= len(args) {
				d := usageDiag("--policy requires a value")
				return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--values":
			if i+1 >= len(args) {
				d := usageDiag("--values requires a value")
				return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--set":
			if i+1 >= len(args) {
				d := usageDiag("--set requires name=value")
				return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--lock":
			if i+1 >= len(args) {
				d := usageDiag("--lock requires a value")
				return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
			}
			lockPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--lock="):
			lockPath = strings.TrimPrefix(a, "--lock=")
		case a == "--locked":
			locked = true
		case a == "--syntax-only":
			syntaxOnly = true
		case a == "--output":
			if i+1 >= len(args) {
				d := usageDiag("--output requires a value")
				return emitValidate(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
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
		d := usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))
		return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		d := usageDiag(fmt.Sprintf("unsupported --output %q (allowed: text, json)", outputMode))
		return emitValidate(stdout, stderr, "text", false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if filePath == "" {
		d := usageDiag("validate requires --file <path>")
		return emitValidate(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if syntaxOnly && locked {
		d := usageDiag("--syntax-only and --locked are mutually exclusive")
		return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitUsage)
	}

	pol := policy.MustDefault()
	if policyPath != "" {
		pdata, err := fs.ReadFile(policyPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read policy: "+err.Error()).
				WithSource(policyPath, "", 0, 0)
			return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitIO)
		}
		parsed, err := policy.Parse(pdata)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, "invalid compile policy: "+err.Error()).
				WithSource(policyPath, "", 0, 0)
			return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitInvalid)
		}
		pol = parsed
	}

	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitValidate(stdout, stderr, outputMode, false, filePath, setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error()).
				WithSource(valuesPath, "", 0, 0)
			return emitValidate(stdout, stderr, outputMode, false, filePath, []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitValidate(stdout, stderr, outputMode, false, filePath, vdiags, ExitInvalid)
		}
	}

	opts := validate.Options{
		SyntaxOnly: syntaxOnly,
		Locked:     locked,
		LockPath:   lockPath,
		Policy:     pol,
		Set:        setMap,
		Values:     values,
	}
	if !syntaxOnly {
		if wsRoot, err := workspace.RootForManifest(filePath); err == nil {
			if ws, err := workspace.New(wsRoot); err == nil {
				opts.Workspace = ws
			}
		}
		// In-memory test doubles have no real workspace directory; full input
		// validation then fails only when the manifest declares resources/builds.
	}

	res := validate.RunWithOptions(fs, filePath, opts)
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitValidate(stdout, stderr, outputMode, res.OK, filePath, res.Diagnostics, code)
}

func emitValidate(stdout, stderr io.Writer, mode string, ok bool, file string, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if ok {
			fmt.Fprintf(stderr, "ok: %s\n", file)
		}
		return code
	}
	var result any
	if ok {
		result = map[string]any{"file": file}
	}
	if err := WriteJSON(stdout, "validate", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
