package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/destroy"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/provider"
)

func runDestroy(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	outputMode := "text"
	instance := ""
	dockerSocket := ""
	containerEngine := ""
	deleteVols := false
	yes := false
	dryRun := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack destroy <project> [--instance <id>] [--delete-volumes] [--yes] [--dry-run] [--container-engine podman|docker] [--docker-socket <path>] [--output text|json]")
			return ExitOK
		case a == "--instance":
			if i+1 >= len(args) {
				return emitDestroy(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--instance requires a value")}, ExitUsage)
			}
			instance = args[i+1]
			i++
		case strings.HasPrefix(a, "--instance="):
			instance = strings.TrimPrefix(a, "--instance=")
		case a == "--delete-volumes":
			deleteVols = true
		case a == "--yes":
			yes = true
		case a == "--dry-run":
			dryRun = true
		case a == "--docker-socket":
			if i+1 >= len(args) {
				return emitDestroy(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--docker-socket requires a value")}, ExitUsage)
			}
			dockerSocket = args[i+1]
			i++
		case strings.HasPrefix(a, "--docker-socket="):
			dockerSocket = strings.TrimPrefix(a, "--docker-socket=")
		case a == "--container-engine":
			if i+1 >= len(args) {
				return emitDestroy(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--container-engine requires podman or docker")}, ExitUsage)
			}
			containerEngine = args[i+1]
			i++
		case strings.HasPrefix(a, "--container-engine="):
			containerEngine = strings.TrimPrefix(a, "--container-engine=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitDestroy(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitDestroy(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitDestroy(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("destroy requires a project")}, ExitUsage)
	}
	prov := provider.HostDestroy{Host: provider.Host{Socket: dockerSocket, Engine: containerEngine}}
	res := destroy.Run(context.Background(), destroy.Options{
		Project: rest[0], Instance: instance, DeleteVolumes: deleteVols, Yes: yes, DryRun: dryRun, Provider: prov,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitDestroy(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitDestroy(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "destroy", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
