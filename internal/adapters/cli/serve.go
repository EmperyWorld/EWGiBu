package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"iacpack/internal/adapters/httpapi"
	"iacpack/internal/diagnostic"
	"iacpack/internal/policy"
)

func runServe(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	workspacePath := ""
	manifestRel := "iacpack.yaml"
	policyPath := ""
	addr := "127.0.0.1:7432"
	outputRel := ".iacpack-out"
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack serve --workspace <dir> [--addr 127.0.0.1:7432] [--manifest iacpack.yaml] [--policy <path>] [--output-root .iacpack-out]")
			fmt.Fprintln(stdout, "  Serves the loopback-only authoring API and embedded Web UI.")
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitServeErr(stderr, usageDiag("--workspace requires a value"))
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--manifest":
			if i+1 >= len(args) {
				return emitServeErr(stderr, usageDiag("--manifest requires a value"))
			}
			manifestRel = args[i+1]
			i++
		case strings.HasPrefix(a, "--manifest="):
			manifestRel = strings.TrimPrefix(a, "--manifest=")
		case a == "--policy":
			if i+1 >= len(args) {
				return emitServeErr(stderr, usageDiag("--policy requires a value"))
			}
			policyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--policy="):
			policyPath = strings.TrimPrefix(a, "--policy=")
		case a == "--addr":
			if i+1 >= len(args) {
				return emitServeErr(stderr, usageDiag("--addr requires a value"))
			}
			addr = args[i+1]
			i++
		case strings.HasPrefix(a, "--addr="):
			addr = strings.TrimPrefix(a, "--addr=")
		case a == "--output-root":
			if i+1 >= len(args) {
				return emitServeErr(stderr, usageDiag("--output-root requires a value"))
			}
			outputRel = args[i+1]
			i++
		case strings.HasPrefix(a, "--output-root="):
			outputRel = strings.TrimPrefix(a, "--output-root=")
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) > 0 {
		return emitServeErr(stderr, usageDiag(fmt.Sprintf("unexpected argument %q", rest[0])))
	}
	if workspacePath == "" {
		return emitServeErr(stderr, usageDiag("serve requires --workspace <dir>"))
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return emitServeErr(stderr, diagnostic.Error(diagnostic.CodeInputIOError, err.Error()))
	}
	pol := policy.MustDefault()
	if policyPath != "" {
		pdata, err := fs.ReadFile(policyPath)
		if err != nil {
			return emitServeErr(stderr, diagnostic.Error(diagnostic.CodeInputIOError, "failed to read policy: "+err.Error()).
				WithSource(policyPath, "", 0, 0))
		}
		parsed, err := policy.Parse(pdata)
		if err != nil {
			return emitServeErr(stderr, diagnostic.Error(diagnostic.CodePolicyUnsupportedField, "invalid compile policy: "+err.Error()).
				WithSource(policyPath, "", 0, 0))
		}
		pol = parsed
	}

	srv, err := httpapi.New(httpapi.Config{
		Workspace:   abs,
		ManifestRel: filepath.ToSlash(manifestRel),
		OutputRel:   filepath.ToSlash(outputRel),
		Policy:      pol,
		Addr:        addr,
	})
	if err != nil {
		return emitServeErr(stderr, diagnostic.Error(diagnostic.CodeCLIInvalidUsage, err.Error()))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(stdout, "iacpack serve listening on http://%s (workspace %s)\n", addr, abs)
	if err := srv.ListenAndServe(ctx); err != nil {
		return emitServeErr(stderr, diagnostic.Error(diagnostic.CodeInternal, err.Error()))
	}
	return ExitOK
}

func emitServeErr(stderr io.Writer, d diagnostic.Diagnostic) int {
	writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
	return mapExitFromDiags([]diagnostic.Diagnostic{d}, ExitUsage)
}
