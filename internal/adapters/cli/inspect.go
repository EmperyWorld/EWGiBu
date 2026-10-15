package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/graph"
	"iacpack/internal/app/inspect"
	"iacpack/internal/app/validate"
	"iacpack/internal/app/verify"
	"iacpack/internal/diagnostic"
	"iacpack/internal/discover"
	"iacpack/internal/params"
	"iacpack/internal/policy"
)

func runInspect(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		d := usageDiag("inspect requires a subcommand (manifest|inputs|graph|bundle)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "manifest":
		return runInspectManifest(args[1:], stdout, stderr, fs)
	case "inputs":
		return runInspectInputs(args[1:], stdout, stderr, fs)
	case "graph":
		return runInspectGraph(args[1:], stdout, stderr, fs)
	case "bundle":
		return runInspectBundle(args[1:], stdout, stderr, fs)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack inspect manifest|inputs|graph --file <path> [--output text|json|mermaid]")
		fmt.Fprintln(stdout, "       iacpack inspect bundle <path> [--output text|json]")
		fmt.Fprintln(stdout, "       iacpack inspect --discover --workspace <path> [--output text|json]")
		return ExitOK
	default:
		// Support: inspect --discover --workspace <path>
		if args[0] == "--discover" || hasFlag(args, "--discover") {
			return runInspectDiscover(args, stdout, stderr, fs)
		}
		d := usageDiag(fmt.Sprintf("unknown inspect subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func runInspectManifest(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	discoverMode := false
	workspacePath := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack inspect manifest --file <path> [--output text|json]")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect manifest", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--discover":
			discoverMode = true
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect manifest", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.TrimPrefix(a, "--output=")
		default:
			rest = append(rest, a)
		}
	}
	if discoverMode {
		return runDiscoverAt(workspacePath, filePath, outputMode, stdout, stderr)
	}
	if len(rest) > 0 {
		return emitInspectCmd(stdout, stderr, "inspect manifest", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitInspectCmd(stdout, stderr, "inspect manifest", "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if filePath == "" {
		return emitInspectCmd(stdout, stderr, "inspect manifest", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("inspect manifest requires --file <path>")}, ExitUsage)
	}

	opts := validate.Options{Policy: policy.MustDefault()}
	if wsRoot, err := workspace.RootForManifest(filePath); err == nil {
		if ws, err := workspace.New(wsRoot); err == nil {
			opts.Workspace = ws
		}
	}
	res := inspect.RunWithOptions(fs, filePath, opts)
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitInspectCmd(stdout, stderr, "inspect manifest", outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func runInspectInputs(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	policyPath := ""
	valuesPath := ""
	var setPairs []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack inspect inputs --file <path> [--policy <path>] [--values <path>] [--set k=v] [--output text|json]")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--policy":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--policy requires a value")}, ExitUsage)
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--values":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--values requires a value")}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--set":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--set requires name=value")}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--output":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitInspect(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if filePath == "" {
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("inspect inputs requires --file <path>")}, ExitUsage)
	}

	pol := policy.MustDefault()
	if policyPath != "" {
		pdata, err := fs.ReadFile(policyPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read policy: "+err.Error())
			return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
		}
		parsed, err := policy.Parse(pdata)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, "invalid compile policy: "+err.Error())
			return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitInvalid)
		}
		pol = parsed
	}
	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitInspect(stdout, stderr, outputMode, false, nil, "", setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error())
			return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitInspect(stdout, stderr, outputMode, false, nil, "", vdiags, ExitInvalid)
		}
	}
	wsRoot, err := workspace.RootForManifest(filePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	ws, err := workspace.New(wsRoot)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	res := inspect.RunInputs(fs, filePath, validate.Options{
		Policy:    pol,
		Set:       setMap,
		Values:    values,
		Workspace: ws,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitInspectCmd(stdout, stderr, "inspect inputs", outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func runInspectDiscover(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	workspacePath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.TrimPrefix(a, "--output=")
		case a == "--discover":
			// ok
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack inspect --discover --workspace <path> [--output text|json]")
			return ExitOK
		default:
			return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	return runDiscoverAt(workspacePath, "", outputMode, stdout, stderr)
}

func runDiscoverAt(workspacePath, filePath, outputMode string, stdout, stderr io.Writer) int {
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitInspect(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	root := workspacePath
	if root == "" && filePath != "" {
		if r, err := workspace.RootForManifest(filePath); err == nil {
			root = r
		} else {
			root = filepath.Dir(filePath)
		}
	}
	if root == "" {
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("discover requires --workspace <path>")}, ExitUsage)
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	ws, err := workspace.New(root)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	sugs, err := discover.Run(ws)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "discover failed: "+err.Error())
		return emitInspect(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	text, model := inspect.FormatDiscover(sugs)
	return emitInspect(stdout, stderr, outputMode, true, model, text, nil, ExitOK)
}

func runInspectGraph(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack inspect graph --file <path> [--output text|json|mermaid]")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect graph", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect graph", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitInspectCmd(stdout, stderr, "inspect graph", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" && outputMode != "mermaid" {
		return emitInspectCmd(stdout, stderr, "inspect graph", "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q (allowed: text, json, mermaid)", outputMode))}, ExitUsage)
	}
	if filePath == "" {
		return emitInspectCmd(stdout, stderr, "inspect graph", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("inspect graph requires --file <path>")}, ExitUsage)
	}
	opts := validate.Options{Policy: policy.MustDefault()}
	if wsRoot, err := workspace.RootForManifest(filePath); err == nil {
		if ws, err := workspace.New(wsRoot); err == nil {
			opts.Workspace = ws
		}
	}
	res := graph.Run(fs, filePath, opts)
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	text := res.Text
	if outputMode == "mermaid" {
		text = res.Mermaid
	}
	model := res.Model
	if model != nil {
		model["mermaid"] = res.Mermaid
	}
	return emitInspectCmd(stdout, stderr, "inspect graph", outputMode, res.OK, model, text, res.Diagnostics, code)
}

func runInspectBundle(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	outputMode := "text"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack inspect bundle <path> [--output text|json]")
			return ExitOK
		case a == "--output":
			if i+1 >= len(args) {
				return emitInspectCmd(stdout, stderr, "inspect bundle", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.TrimPrefix(a, "--output=")
		default:
			rest = append(rest, a)
		}
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitInspectCmd(stdout, stderr, "inspect bundle", "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitInspectCmd(stdout, stderr, "inspect bundle", outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("inspect bundle requires exactly one path")}, ExitUsage)
	}
	res := verify.Inspect(rest[0])
	code := ExitOK
	if !res.OK {
		code = ExitBundle
		for _, d := range res.Diagnostics {
			if d.Code == diagnostic.CodeInputIOError {
				code = ExitIO
				break
			}
		}
	}
	return emitInspectCmd(stdout, stderr, "inspect bundle", outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitInspect(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
	return emitInspectCmd(stdout, stderr, "inspect", mode, ok, model, text, diags, code)
}

func emitInspectCmd(stdout, stderr io.Writer, command, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if ok {
			_, _ = io.WriteString(stdout, text)
		}
		return code
	}
	var result any
	if ok {
		result = model
	}
	if err := WriteJSON(stdout, command, ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
