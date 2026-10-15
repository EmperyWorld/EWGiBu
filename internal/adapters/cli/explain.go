package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/explain"
	"iacpack/internal/diagnostic"
)

func runExplain(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	outputMode := "text"
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack explain <bundle> <service> [--output text|json]")
			fmt.Fprintln(stdout, "  Shows image, gpu, ldLibraryPath, and devices for one service. No secret values.")
			return ExitOK
		case a == "--output":
			if i+1 >= len(args) {
				return emitExplain(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitExplain(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 2 {
		return emitExplain(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("explain requires a bundle and a service id")}, ExitUsage)
	}
	res := explain.Run(rest[0], rest[1])
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
		for _, d := range res.Diagnostics {
			if strings.HasPrefix(d.Code, "BUNDLE.") {
				code = ExitBundle
				break
			}
		}
	}
	return emitExplain(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitExplain(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "explain", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
