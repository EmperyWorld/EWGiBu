package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/inspect"
	"iacpack/internal/app/projectinit"
	"iacpack/internal/diagnostic"
	"iacpack/internal/discover"
)

func runInit(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	outPath := projectinit.DefaultOutputName
	discoverMode := false
	workspacePath := ""
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack init [--output <path>] [--output json]")
			fmt.Fprintln(stdout, "       iacpack init --discover --workspace <path> [--output text|json]")
			fmt.Fprintln(stdout, "  Creates a minimal iacpack.yaml (atomic create; refuses overwrite).")
			fmt.Fprintln(stdout, "  --discover prints reviewable suggestions only; it does not change build inputs.")
			return ExitOK
		case a == "--discover":
			discoverMode = true
		case a == "--workspace":
			if i+1 >= len(args) {
				d := usageDiag("--workspace requires a value")
				return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--output":
			if i+1 >= len(args) {
				d := usageDiag("--output requires a value")
				return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
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
	if discoverMode {
		return runInitDiscover(workspacePath, outputMode, stdout, stderr)
	}
	if len(rest) > 0 {
		d := usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if outPath == "" {
		d := usageDiag("output path must not be empty")
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
	}

	exists, err := fs.Exists(outPath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to check output path: "+err.Error()).
			WithSource(outPath, "", 0, 0)
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	if exists {
		d := diagnostic.Error(diagnostic.CodeInputIOError,
			fmt.Sprintf("refusing to overwrite existing file %s", outPath)).
			WithSource(outPath, "", 0, 0).
			WithHint("Choose a different --output path")
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitIO)
	}

	data := projectinit.Content()
	if err := fs.WriteFileAtomic(outPath, data, 0o644); err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to write manifest: "+err.Error()).
			WithSource(outPath, "", 0, 0)
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitIO)
	}

	if outputMode == "text" {
		fmt.Fprintf(stderr, "wrote %s\n", filepath.Clean(outPath))
	}
	return emitInit(stdout, stderr, outputMode, true, outPath, nil, ExitOK)
}

func runInitDiscover(workspacePath, outputMode string, stdout, stderr io.Writer) int {
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		d := usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))
		return emitInit(stdout, stderr, "text", false, "", []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if workspacePath == "" {
		d := usageDiag("init --discover requires --workspace <path>")
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if abs, err := filepath.Abs(workspacePath); err == nil {
		workspacePath = abs
	}
	ws, err := workspace.New(workspacePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to open workspace: "+err.Error())
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	sugs, err := discover.Run(ws)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "discover failed: "+err.Error())
		return emitInit(stdout, stderr, outputMode, false, "", []diagnostic.Diagnostic{d}, ExitIO)
	}
	text, model := inspect.FormatDiscover(sugs)
	if outputMode != "json" {
		_, _ = io.WriteString(stdout, text)
		return ExitOK
	}
	if err := WriteJSON(stdout, "init discover", true, model, []any{}); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	return ExitOK
}

func emitInit(stdout, stderr io.Writer, mode string, ok bool, path string, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		return code
	}
	var result any
	if ok {
		result = map[string]any{"path": path}
	}
	if err := WriteJSON(stdout, "init", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
