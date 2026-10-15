package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/render"
	"iacpack/internal/diagnostic"
	"iacpack/internal/params"
)

func runRender(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		d := usageDiag("render requires a subcommand (compose)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "compose":
		return runRenderCompose(args[1:], stdout, stderr, fs)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack render compose <bundle> [--set k=v] [--values <path>] [--output <path>|text|json]")
		return ExitOK
	default:
		d := usageDiag(fmt.Sprintf("unknown render subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func runRenderCompose(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	outPath := ""
	valuesPath := ""
	var setPairs []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack render compose <bundle> [--set k=v] [--values <path>] [--output <path>|json]")
			fmt.Fprintln(stdout, "  Renders Compose from a verified bundle deployment document.")
			return ExitOK
		case a == "--set":
			if i+1 >= len(args) {
				return emitRender(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--set requires name=value")}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--values":
			if i+1 >= len(args) {
				return emitRender(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--values requires a value")}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitRender(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitRender(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported machine --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitRender(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("render compose requires exactly one bundle path")}, ExitUsage)
	}
	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitRender(stdout, stderr, outputMode, false, nil, "", setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error())
			return emitRender(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitRender(stdout, stderr, outputMode, false, nil, "", vdiags, ExitInvalid)
		}
	}
	res := render.Compose(fsWriter{fs}, render.Options{
		Bundle: rest[0], Set: setMap, Values: values, Output: outPath,
	})
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
	// For text mode without --output path, print compose YAML to stdout.
	text := res.Text
	if outputMode == "text" && outPath != "" && res.OK {
		text = fmt.Sprintf("wrote compose to %s (%d bytes)\n", outPath, len(res.Bytes))
	}
	return emitRender(stdout, stderr, outputMode, res.OK, res.Model, text, res.Diagnostics, code)
}

func emitRender(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "render compose", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
