package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/app/diff"
	"iacpack/internal/diagnostic"
	"iacpack/internal/policy"
)

func runDiff(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	from, to := "", ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack diff --from <source> --to <source> [--output text|json]")
			fmt.Fprintln(stdout, "  Compares two explicit manifest, lock, or bundle sources.")
			return ExitOK
		case a == "--from":
			if i+1 >= len(args) {
				return emitDiff(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--from requires a value")}, ExitUsage)
			}
			from = args[i+1]
			i++
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case a == "--to":
			if i+1 >= len(args) {
				return emitDiff(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--to requires a value")}, ExitUsage)
			}
			to = args[i+1]
			i++
		case strings.HasPrefix(a, "--to="):
			to = strings.TrimPrefix(a, "--to=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitDiff(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitDiff(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitDiff(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if from == "" || to == "" {
		return emitDiff(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("diff requires --from and --to")}, ExitUsage)
	}

	var wsRoot string
	for _, p := range []string{from, to} {
		if strings.HasSuffix(strings.ToLower(p), ".yaml") || strings.HasSuffix(strings.ToLower(p), ".yml") {
			wsRoot = filepath.Dir(p)
			break
		}
	}
	opts := diff.Options{From: from, To: to, Policy: policy.MustDefault()}
	if wsRoot != "" {
		if abs, err := filepath.Abs(wsRoot); err == nil {
			wsRoot = abs
		}
		if ws, err := workspace.New(wsRoot); err == nil {
			opts.Workspace = ws
		}
	}
	res := diff.Run(fs, opts)
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
		for _, d := range res.Diagnostics {
			if d.Code == diagnostic.CodeCLIUnsupportedSource {
				code = ExitInvalid
				break
			}
			if strings.HasPrefix(d.Code, "BUNDLE.") {
				code = ExitBundle
				break
			}
		}
	}
	return emitDiff(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitDiff(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "diff", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
