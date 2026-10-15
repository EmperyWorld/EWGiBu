package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/taskrun"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/provider"
)

func runTask(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	if len(args) == 0 {
		d := usageDiag("task requires a subcommand (run)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "run":
		return runTaskRun(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack task run <project> <task> [--instance <id>] [--yes] [--container-engine podman|docker] [--docker-socket <path>] [--output text|json]")
		return ExitOK
	default:
		d := usageDiag(fmt.Sprintf("unknown task subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func runTaskRun(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	instance := ""
	dockerSocket := ""
	containerEngine := ""
	yes := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack task run <project> <task> [--instance <id>] [--yes] [--container-engine podman|docker] [--docker-socket <path>] [--output text|json]")
			return ExitOK
		case a == "--instance":
			if i+1 >= len(args) {
				return emitTask(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--instance requires a value")}, ExitUsage)
			}
			instance = args[i+1]
			i++
		case strings.HasPrefix(a, "--instance="):
			instance = strings.TrimPrefix(a, "--instance=")
		case a == "--yes":
			yes = true
		case a == "--docker-socket":
			if i+1 >= len(args) {
				return emitTask(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--docker-socket requires a value")}, ExitUsage)
			}
			dockerSocket = args[i+1]
			i++
		case strings.HasPrefix(a, "--docker-socket="):
			dockerSocket = strings.TrimPrefix(a, "--docker-socket=")
		case a == "--container-engine":
			if i+1 >= len(args) {
				return emitTask(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--container-engine requires podman or docker")}, ExitUsage)
			}
			containerEngine = args[i+1]
			i++
		case strings.HasPrefix(a, "--container-engine="):
			containerEngine = strings.TrimPrefix(a, "--container-engine=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitTask(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitTask(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 2 {
		return emitTask(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("task run requires <project> <task>")}, ExitUsage)
	}
	prov := provider.HostApply{Host: provider.Host{Socket: dockerSocket, Engine: containerEngine}}
	res := taskrun.Run(context.Background(), taskrun.Options{
		Project: rest[0], Task: rest[1], Instance: instance, Yes: yes,
		Provider: prov,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitTask(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitTask(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "task run", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
