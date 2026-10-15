package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/diagnostic"
)

// Execute runs the root command and returns a process exit code.
func Execute() int {
	return ExecuteWith(os.Args[1:], os.Stdout, os.Stderr)
}

// ExecuteWith runs the CLI with injected args and streams (for tests).
func ExecuteWith(args []string, stdout, stderr io.Writer) int {
	return ExecuteWithFS(args, stdout, stderr, filesystem.OS{})
}

// FileSystem is the narrow FS surface used by CLI commands.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	Exists(path string) (bool, error)
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
	WriteFileAtomicReplace(path string, data []byte, perm os.FileMode) error
}

// ExecuteWithFS runs the CLI with an injected filesystem.
func ExecuteWithFS(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return ExitUsage
	}
	switch args[0] {
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr, fs)
	case "validate":
		return runValidate(args[1:], stdout, stderr, fs)
	case "lock":
		return runLock(args[1:], stdout, stderr, fs)
	case "inspect":
		return runInspect(args[1:], stdout, stderr, fs)
	case "build":
		return runBuild(args[1:], stdout, stderr, fs)
	case "images":
		return runImages(args[1:], stdout, stderr, fs)
	case "verify":
		return runVerify(args[1:], stdout, stderr, fs)
	case "diff":
		return runDiff(args[1:], stdout, stderr, fs)
	case "render":
		return runRender(args[1:], stdout, stderr, fs)
	case "manifest":
		return runManifest(args[1:], stdout, stderr, fs)
	case "plan":
		return runPlan(args[1:], stdout, stderr, fs)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, fs)
	case "apply":
		return runApply(args[1:], stdout, stderr, fs)
	case "bundle":
		return runBundle(args[1:], stdout, stderr, fs)
	case "explain":
		return runExplain(args[1:], stdout, stderr, fs)
	case "status":
		return runStatus(args[1:], stdout, stderr, fs)
	case "destroy":
		return runDestroy(args[1:], stdout, stderr, fs)
	case "resume":
		return runResume(args[1:], stdout, stderr, fs)
	case "task":
		return runTask(args[1:], stdout, stderr, fs)
	case "schema":
		return runSchema(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr, fs)
	case "import":
		return runImport(args[1:], stdout, stderr)
	case "release":
		return runRelease(args[1:], stdout, stderr, fs)
	case "help", "-h", "--help":
		printRootUsage(stdout)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printRootUsage(stderr)
		return ExitUsage
	}
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: iacpack <command> [flags]")
	fmt.Fprintln(w, "commands: init, validate, lock, images fetch, build, verify, render compose,")
	fmt.Fprintln(w, "          inspect manifest|inputs|graph|bundle, diff, manifest migrate, plan, doctor,")
	fmt.Fprintln(w, "          apply, bundle, explain, status, resume, destroy, task run, import propose,")
	fmt.Fprintln(w, "          release init-templates, serve, schema export, version")
}

func parseOutputFlag(args []string) (output string, rest []string, err error) {
	output = "text"
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--output":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--output requires a value")
			}
			output = args[i+1]
			i++
		case strings.HasPrefix(a, "--output="):
			output = strings.TrimPrefix(a, "--output=")
		case a == "-h" || a == "--help":
			return output, append(rest, a), nil
		default:
			rest = append(rest, a)
		}
	}
	return output, rest, nil
}

func writeDiagnosticsText(stderr io.Writer, diags []diagnostic.Diagnostic) {
	if len(diags) == 0 {
		return
	}
	_, _ = io.WriteString(stderr, diagnostic.FormatText(diags))
}

func usageDiag(msg string) diagnostic.Diagnostic {
	return diagnostic.Error(diagnostic.CodeCLIInvalidUsage, msg)
}

func mapExitFromDiags(diags []diagnostic.Diagnostic, fallback int) int {
	for _, d := range diags {
		switch d.Code {
		case diagnostic.CodeCLIInvalidUsage:
			return ExitUsage
		case diagnostic.CodeInputIOError:
			return ExitIO
		case diagnostic.CodeInternal:
			return ExitInternal
		}
	}
	if diagnostic.HasError(diags) {
		return fallback
	}
	return ExitOK
}
