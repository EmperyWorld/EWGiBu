package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/images"
	"iacpack/internal/diagnostic"
)

func runImages(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		d := usageDiag("images requires a subcommand (fetch)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "fetch":
		return runImagesFetch(args[1:], stdout, stderr, fs)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack images fetch --lock <path> --output <dir> [--output text|json]")
		return ExitOK
	default:
		d := usageDiag(fmt.Sprintf("unknown images subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func runImagesFetch(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	lockPath, outPath := "", ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack images fetch --lock <path> --output <dir> [--output text|json]")
			fmt.Fprintln(stdout, "  Network-enabled: materializes a verified OCI layout cache from lock digests.")
			return ExitOK
		case a == "--lock":
			if i+1 >= len(args) {
				return emitImages(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--lock requires a value")}, ExitUsage)
			}
			lockPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--lock="):
			lockPath = strings.TrimPrefix(a, "--lock=")
		case a == "--file":
			// accepted for TZ symmetry; lock is authoritative for digests
			if i+1 >= len(args) {
				return emitImages(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			i++
		case strings.HasPrefix(a, "--file="):
			// ignore value; lock drives fetch
		case a == "--output":
			if i+1 >= len(args) {
				return emitImages(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitImages(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitImages(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported machine --output %q", outputMode))}, ExitUsage)
	}
	// Production Docker fetcher is optional; without docker, require tests to inject.
	// CLI uses a denied fetcher unless IACP_IMAGES_FETCHER=fixture is unset — use DockerFetcher stub that errors with hint.
	res := images.Fetch(context.Background(), fs, images.Options{
		LockPath: lockPath, Output: outPath, Fetcher: images.DockerCLIFetcher{},
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitIO)
	}
	return emitImages(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitImages(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "images fetch", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
