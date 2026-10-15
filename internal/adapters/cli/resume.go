package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/resume"
	"iacpack/internal/diagnostic"
)

func runResume(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	outputMode := "text"
	instance := ""
	dryRun := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack resume <project> [--instance <id>] [--dry-run] [--output text|json]")
			return ExitOK
		case a == "--instance":
			if i+1 >= len(args) {
				return emitResume(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--instance requires a value")}, ExitUsage)
			}
			instance = args[i+1]
			i++
		case strings.HasPrefix(a, "--instance="):
			instance = strings.TrimPrefix(a, "--instance=")
		case a == "--dry-run":
			dryRun = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitResume(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitResume(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitResume(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("resume requires a project")}, ExitUsage)
	}
	res := resume.Run(resume.Options{Project: rest[0], Instance: instance, DryRun: dryRun})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitPartial)
	}
	return emitResume(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitResume(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "resume", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
