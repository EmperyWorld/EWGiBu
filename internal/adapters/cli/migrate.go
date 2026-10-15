package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"iacpack/internal/app/migrate"
	"iacpack/internal/diagnostic"
)

func runManifest(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		d := usageDiag("manifest requires a subcommand (migrate)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "migrate":
		return runManifestMigrate(args[1:], stdout, stderr, fs)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack manifest migrate --file <path> --to <apiVersion> --output <path> [--output text|json]")
		return ExitOK
	default:
		d := usageDiag(fmt.Sprintf("unknown manifest subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func runManifestMigrate(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	filePath, to, outPath := "", "", ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack manifest migrate --file <path> --to <apiVersion> --output <path> [--output text|json]")
			fmt.Fprintln(stdout, "  Explicit migration only; never rewrites in place. Unsupported pairs fail.")
			return ExitOK
		case a == "--file":
			if i+1 >= len(args) {
				return emitMigrate(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--file requires a value")}, ExitUsage)
			}
			filePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			filePath = strings.TrimPrefix(a, "--file=")
		case a == "--to":
			if i+1 >= len(args) {
				return emitMigrate(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--to requires a value")}, ExitUsage)
			}
			to = args[i+1]
			i++
		case strings.HasPrefix(a, "--to="):
			to = strings.TrimPrefix(a, "--to=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitMigrate(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitMigrate(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitMigrate(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported machine --output %q", outputMode))}, ExitUsage)
	}
	res := migrate.Run(fs, fsWriter{fs}, migrate.Options{File: filePath, To: to, Output: outPath})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitMigrate(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

type fsWriter struct{ fs FileSystem }

func (w fsWriter) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return w.fs.WriteFileAtomic(path, data, perm)
}

func emitMigrate(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "manifest migrate", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
