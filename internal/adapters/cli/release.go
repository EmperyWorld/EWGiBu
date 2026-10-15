package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"iacpack/internal/app/releasetemplates"
	"iacpack/internal/diagnostic"
)

func runRelease(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: iacpack release init-templates --workspace <path> [--force] [--dry-run] [--output text|json]")
		return ExitUsage
	}
	switch args[0] {
	case "init-templates":
		return runReleaseInitTemplates(args[1:], stdout, stderr, fs)
	case "-h", "--help":
		fmt.Fprintln(stdout, "usage: iacpack release init-templates --workspace <path> [--force] [--dry-run] [--output text|json]")
		fmt.Fprintln(stdout, "  Writes opted-in GitHub CI/release helper files. Never pushes or publishes (ADR-019).")
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown release subcommand %q\n", args[0])
		return ExitUsage
	}
}

func runReleaseInitTemplates(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	_ = fs
	outputMode := "text"
	workspacePath := ""
	force := false
	dryRun := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack release init-templates --workspace <path> [--force] [--dry-run] [--output text|json]")
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitReleaseInit(stdout, stderr, outputMode, releasetemplates.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--workspace requires a value")},
				}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--force":
			force = true
		case a == "--dry-run":
			dryRun = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitReleaseInit(stdout, stderr, outputMode, releasetemplates.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--output requires a value")},
				}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitReleaseInit(stdout, stderr, outputMode, releasetemplates.Result{
				Diagnostics: []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))},
			}, ExitUsage)
		}
	}

	if outputMode != "text" && outputMode != "json" {
		return emitReleaseInit(stdout, stderr, "text", releasetemplates.Result{
			Diagnostics: []diagnostic.Diagnostic{usageDiag("--output must be text or json")},
		}, ExitUsage)
	}
	if workspacePath == "" {
		return emitReleaseInit(stdout, stderr, outputMode, releasetemplates.Result{
			Diagnostics: []diagnostic.Diagnostic{usageDiag("release init-templates requires --workspace <path>")},
		}, ExitUsage)
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "workspace: "+err.Error())
		return emitReleaseInit(stdout, stderr, outputMode, releasetemplates.Result{Diagnostics: []diagnostic.Diagnostic{d}}, ExitIO)
	}

	var res releasetemplates.Result
	if dryRun {
		res = releasetemplates.Propose(releasetemplates.Request{Workspace: abs, Force: force})
	} else {
		res = releasetemplates.Write(releasetemplates.Request{Workspace: abs, Force: force})
	}
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitReleaseInit(stdout, stderr, outputMode, res, code)
}

func emitReleaseInit(stdout, stderr io.Writer, mode string, res releasetemplates.Result, code int) int {
	files := res.Files
	if files == nil {
		files = []releasetemplates.FilePlan{}
	}
	result := map[string]any{
		"force": res.Force,
		"wrote": res.Wrote,
		"files": files,
	}
	if mode == "json" {
		if err := WriteJSON(stdout, "release init-templates", res.OK, result, diagnostic.ToMaps(res.Diagnostics)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, res.Diagnostics)
	if res.OK {
		verb := "proposed"
		if res.Wrote {
			verb = "wrote"
		}
		fmt.Fprintf(stdout, "ok: %s %d release template file(s)\n", verb, len(files))
		for _, f := range files {
			fmt.Fprintf(stdout, "  %s %s (%d bytes)\n", f.Action, f.Path, f.Bytes)
		}
	} else {
		fmt.Fprintln(stdout, "release init-templates failed")
	}
	return code
}
