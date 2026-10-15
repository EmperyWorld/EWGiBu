package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/apply"
	"iacpack/internal/diagnostic"
	"iacpack/internal/params"
	"iacpack/internal/runtime/oppolicy"
	"iacpack/internal/runtime/provider"
	"iacpack/internal/runtime/secrets"
)

func runApply(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	instance := ""
	opPolicyPath := ""
	valuesPath := ""
	workspace := ""
	dockerSocket := ""
	containerEngine := ""
	yes := false
	allowDev := false
	secretsDir := ""
	var setPairs, secretPairs []string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack apply <bundle|kit-dir> [--instance <id>] [--workspace <dir>] [--set k=v] [--values <path>] [--secrets-dir <dir>] [--secret-file id=path] [--operator-policy <path>] [--container-engine podman|docker] [--docker-socket <path>] [--yes] [--output text|json]")
			fmt.Fprintln(stdout, "  A portable kit directory (images.txt + host-paths.json) is checked before containers start.")
			fmt.Fprintln(stdout, "  --secrets-dir reads one file per secret id. --secret-file overrides the same id.")
			fmt.Fprintln(stdout, "  Before any state write, apply reports images it will pull, local images pull cannot fetch,")
			fmt.Fprintln(stdout, "  missing host paths, missing secret files, and deploy parameters with no value.")
			fmt.Fprintln(stdout, "  Default container engine: Podman when available, else Docker (override: --container-engine or IACPACK_CONTAINER_ENGINE).")
			return ExitOK
		case a == "--instance":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--instance requires a value")}, ExitUsage)
			}
			instance = args[i+1]
			i++
		case strings.HasPrefix(a, "--instance="):
			instance = strings.TrimPrefix(a, "--instance=")
		case a == "--yes":
			yes = true
		case a == "--allow-development-build":
			allowDev = true
		case a == "--operator-policy":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--operator-policy requires a value")}, ExitUsage)
			}
			opPolicyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--operator-policy="):
			opPolicyPath = strings.TrimPrefix(a, "--operator-policy=")
		case a == "--set":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--set requires name=value")}, ExitUsage)
			}
			setPairs = append(setPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--set="):
			setPairs = append(setPairs, strings.TrimPrefix(a, "--set="))
		case a == "--values":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--values requires a value")}, ExitUsage)
			}
			valuesPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--values="):
			valuesPath = strings.TrimPrefix(a, "--values=")
		case a == "--secrets-dir":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--secrets-dir requires a directory")}, ExitUsage)
			}
			secretsDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--secrets-dir="):
			secretsDir = strings.TrimPrefix(a, "--secrets-dir=")
		case a == "--secret-file":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--secret-file requires id=path")}, ExitUsage)
			}
			secretPairs = append(secretPairs, args[i+1])
			i++
		case strings.HasPrefix(a, "--secret-file="):
			secretPairs = append(secretPairs, strings.TrimPrefix(a, "--secret-file="))
		case a == "--docker-socket":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--docker-socket requires a value")}, ExitUsage)
			}
			dockerSocket = args[i+1]
			i++
		case strings.HasPrefix(a, "--docker-socket="):
			dockerSocket = strings.TrimPrefix(a, "--docker-socket=")
		case a == "--container-engine":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--container-engine requires podman or docker")}, ExitUsage)
			}
			containerEngine = args[i+1]
			i++
		case strings.HasPrefix(a, "--container-engine="):
			containerEngine = strings.TrimPrefix(a, "--container-engine=")
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspace = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspace = strings.TrimPrefix(a, "--workspace=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitApply(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("apply requires exactly one bundle path")}, ExitUsage)
	}
	setMap, setDiags := params.ParseSetPairs(setPairs)
	if diagnostic.HasError(setDiags) {
		return emitApply(stdout, stderr, outputMode, false, nil, "", setDiags, ExitInvalid)
	}
	var values map[string]any
	if valuesPath != "" {
		raw, err := fs.ReadFile(valuesPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read values: "+err.Error())
			return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
		}
		var vdiags []diagnostic.Diagnostic
		values, vdiags = params.ParseValuesJSON(raw)
		if diagnostic.HasError(vdiags) {
			return emitApply(stdout, stderr, outputMode, false, nil, "", vdiags, ExitInvalid)
		}
	}
	op := oppolicy.Default()
	if opPolicyPath != "" {
		raw, err := fs.ReadFile(opPolicyPath)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read operator policy: "+err.Error())
			return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
		}
		parsed, err := oppolicy.Parse(raw)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, err.Error())
			return emitApply(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitInvalid)
		}
		op = parsed
	}
	secretPairs, secretDiags := secrets.Expand(secretsDir, secretPairs)
	if diagnostic.HasError(secretDiags) {
		return emitApply(stdout, stderr, outputMode, false, nil, "", secretDiags, ExitInvalid)
	}
	prov := provider.HostApply{Host: provider.Host{Socket: dockerSocket, Engine: containerEngine}}
	res := apply.Run(context.Background(), apply.Options{
		Bundle: rest[0], Instance: instance, Set: setMap, Values: values,
		SecretFiles: secretPairs, OperatorPolicy: op, Yes: yes, AllowDevBuild: allowDev && yes,
		Workspace: workspace,
		Provider:  prov,
	})
	code := ExitOK
	if !res.OK {
		if res.ExitHint == 7 || res.Mutated {
			code = ExitPartial
		} else if res.ExitHint == 6 {
			code = ExitPreflight
		} else {
			code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
		}
	}
	return emitApply(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitApply(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "apply", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
