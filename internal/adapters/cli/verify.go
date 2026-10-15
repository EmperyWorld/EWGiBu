package cli

import (
	"fmt"
	"io"
	"strings"

	"iacpack/internal/app/verify"
	"iacpack/internal/diagnostic"
	"iacpack/internal/runtime/oppolicy"
	"iacpack/internal/supplychain"
)

func runVerify(args []string, stdout, stderr io.Writer, fs FileSystem) int {
	outputMode := "text"
	sigPath, rootPath, idPath, opPolicyPath := "", "", "", ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack verify <bundle> [--signature <path>] [--trusted-root <path>] [--identity-policy <path>] [--operator-policy <path>] [--output text|json]")
			fmt.Fprintln(stdout, "  Safely verifies a .iacbundle; optional offline detached signature checks (no network).")
			return ExitOK
		case a == "--signature":
			if i+1 >= len(args) {
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--signature requires a value")}, ExitUsage)
			}
			sigPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--signature="):
			sigPath = strings.TrimPrefix(a, "--signature=")
		case a == "--trusted-root":
			if i+1 >= len(args) {
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--trusted-root requires a value")}, ExitUsage)
			}
			rootPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--trusted-root="):
			rootPath = strings.TrimPrefix(a, "--trusted-root=")
		case a == "--identity-policy":
			if i+1 >= len(args) {
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--identity-policy requires a value")}, ExitUsage)
			}
			idPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--identity-policy="):
			idPath = strings.TrimPrefix(a, "--identity-policy=")
		case a == "--operator-policy":
			if i+1 >= len(args) {
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--operator-policy requires a value")}, ExitUsage)
			}
			opPolicyPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--operator-policy="):
			opPolicyPath = strings.TrimPrefix(a, "--operator-policy=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
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
		return emitVerify(stdout, stderr, "text", false, nil, "", []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unsupported --output %q", outputMode))}, ExitUsage)
	}
	if len(rest) != 1 {
		return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{usageDiag("verify requires exactly one bundle path")}, ExitUsage)
	}
	res := verify.Run(rest[0])
	// Signature checks are opt-in via --signature / --operator-policy (release path).
	// Archive-only verify remains the default for local builds.
	if sigPath != "" || opPolicyPath != "" {
		allowUnsigned := false
		if opPolicyPath != "" {
			raw, err := fs.ReadFile(opPolicyPath)
			if err != nil {
				d := diagnostic.Error(diagnostic.CodeInputIOError, "failed to read operator policy: "+err.Error())
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitIO)
			}
			op, err := oppolicy.Parse(raw)
			if err != nil {
				d := diagnostic.Error(diagnostic.CodePolicyUnsupportedField, err.Error())
				return emitVerify(stdout, stderr, outputMode, false, nil, "", []diagnostic.Diagnostic{d}, ExitInvalid)
			}
			allowUnsigned = op.AllowUnsigned
		}
		vr := supplychain.VerifyPaths(supplychain.VerifyOptions{
			ArtifactPath: rest[0], EnvelopePath: sigPath, TrustedRootPath: rootPath,
			IdentityPath: idPath, AllowUnsigned: allowUnsigned && sigPath == "",
		})
		if !vr.OK {
			res.OK = false
			res.Diagnostics = append(res.Diagnostics, diagnostic.Error("BUNDLE.SIGNATURE", vr.Error))
		} else if res.Model != nil {
			res.Model["signed"] = vr.Signed
			if vr.Identity != "" {
				res.Model["signerIdentity"] = vr.Identity
			}
		}
	}
	code := ExitOK
	if !res.OK {
		code = ExitBundle
		for _, d := range res.Diagnostics {
			if d.Code == diagnostic.CodeInputIOError {
				code = ExitIO
				break
			}
			if d.Code == diagnostic.CodeCLIInvalidUsage {
				code = ExitUsage
				break
			}
		}
	}
	return emitVerify(stdout, stderr, outputMode, res.OK, res.Model, res.Text, res.Diagnostics, code)
}

func emitVerify(stdout, stderr io.Writer, mode string, ok bool, model map[string]any, text string, diags []diagnostic.Diagnostic, code int) int {
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
	if err := WriteJSON(stdout, "verify", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
