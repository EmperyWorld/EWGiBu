package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/doctor"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/provider"
)

func runDoctor(args []string, stdout, stderr io.Writer, _ FileSystem) int {
	outputMode := "text"
	bundle := ""
	dockerSocket := ""
	containerEngine := ""
	dockerConfig := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack doctor [--bundle <path>] [--container-engine podman|docker] [--docker-socket <path>] [--docker-config <dir>] [--output text|json]")
			fmt.Fprintln(stdout, "  Read-only container-engine/host preflight (default Podman, else Docker). No mutations.")
			return ExitOK
		case a == "--bundle":
			if i+1 >= len(args) {
				return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--bundle requires a value")}, ExitUsage)
			}
			bundle = args[i+1]
			i++
		case strings.HasPrefix(a, "--bundle="):
			bundle = strings.TrimPrefix(a, "--bundle=")
		case a == "--docker-socket":
			if i+1 >= len(args) {
				return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--docker-socket requires a value")}, ExitUsage)
			}
			dockerSocket = args[i+1]
			i++
		case strings.HasPrefix(a, "--docker-socket="):
			dockerSocket = strings.TrimPrefix(a, "--docker-socket=")
		case a == "--container-engine":
			if i+1 >= len(args) {
				return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--container-engine requires podman or docker")}, ExitUsage)
			}
			containerEngine = args[i+1]
			i++
		case strings.HasPrefix(a, "--container-engine="):
			containerEngine = strings.TrimPrefix(a, "--container-engine=")
		case a == "--docker-config":
			if i+1 >= len(args) {
				return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--docker-config requires a value")}, ExitUsage)
			}
			dockerConfig = args[i+1]
			i++
		case strings.HasPrefix(a, "--docker-config="):
			dockerConfig = strings.TrimPrefix(a, "--docker-config=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitDoctor(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", rest[0]))}, ExitUsage)
	}
	outputMode = strings.ToLower(outputMode)
	if outputMode != "text" && outputMode != "json" {
		return emitDoctor(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	res := doctor.Run(context.Background(), doctor.Options{
		Bundle: bundle, DockerSocket: dockerSocket, ContainerEngine: containerEngine, DockerConfig: dockerConfig,
		Provider: provider.Host{Socket: dockerSocket, Engine: containerEngine},
	})
	code := ExitOK
	if !res.OK {
		code = ExitPreflight
		for _, d := range res.Diagnostics {
			if d.Code == diagnostic.CodeCLIInvalidUsage {
				code = ExitUsage
				break
			}
			if strings.HasPrefix(d.Code, "BUNDLE.") {
				code = ExitBundle
				break
			}
		}
	}
	return emitDoctor(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitDoctor(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		writeDiagnosticsText(stderr, diags)
		if ok || text != "" {
			_, _ = io.WriteString(stdout, text)
		}
		return code
	}
	var result any
	if model != nil {
		result = model
	}
	if err := WriteJSON(stdout, "doctor", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
