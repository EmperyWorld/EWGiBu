package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/schemaembed"
)

func runSchema(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		d := usageDiag("schema requires a subcommand (export)")
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
	switch args[0] {
	case "export":
		return runSchemaExport(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: iacpack schema export [--output text|json]")
		return ExitOK
	default:
		d := usageDiag(fmt.Sprintf("unknown schema subcommand %q", args[0]))
		writeDiagnosticsText(stderr, []diagnostic.Diagnostic{d})
		return ExitUsage
	}
}

func runSchemaExport(args []string, stdout, stderr io.Writer) int {
	output, rest, err := parseOutputFlag(args)
	if err != nil {
		d := usageDiag(err.Error())
		return emitSchemaExport(stdout, stderr, "text", false, nil, []diagnostic.Diagnostic{d}, ExitUsage)
	}
	if len(rest) > 0 {
		a := rest[0]
		if a == "-h" || a == "--help" {
			fmt.Fprintln(stdout, "usage: iacpack schema export [--output text|json]")
			return ExitOK
		}
		d := usageDiag(fmt.Sprintf("unexpected argument %q", a))
		return emitSchemaExport(stdout, stderr, output, false, nil, []diagnostic.Diagnostic{d}, ExitUsage)
	}
	output = strings.ToLower(output)
	if output != "text" && output != "json" {
		d := usageDiag(fmt.Sprintf("unsupported --output %q (allowed: text, json)", output))
		return emitSchemaExport(stdout, stderr, "text", false, nil, []diagnostic.Diagnostic{d}, ExitUsage)
	}

	raw := schemaembed.ManifestV1Alpha1()
	var schemaObj any
	if err := json.Unmarshal(raw, &schemaObj); err != nil {
		d := diagnostic.Error(diagnostic.CodeInternal, "embedded manifest schema is corrupt")
		return emitSchemaExport(stdout, stderr, output, false, nil, []diagnostic.Diagnostic{d}, ExitInternal)
	}
	return emitSchemaExport(stdout, stderr, output, true, schemaObj, nil, ExitOK)
}

func emitSchemaExport(stdout, stderr io.Writer, mode string, ok bool, schema any, diags []diagnostic.Diagnostic, code int) int {
	diagnostic.SortDeterministic(diags)
	if mode != "json" {
		if ok {
			raw := schemaembed.ManifestV1Alpha1()
			if _, err := stdout.Write(raw); err != nil {
				fmt.Fprintln(stderr, err.Error())
				return ExitInternal
			}
			if len(raw) == 0 || raw[len(raw)-1] != '\n' {
				fmt.Fprintln(stdout)
			}
		}
		writeDiagnosticsText(stderr, diags)
		return code
	}
	var result any
	if ok {
		result = map[string]any{
			"id":     "iacpack.schemas.manifest.v1alpha1",
			"schema": schema,
		}
	}
	if err := WriteJSON(stdout, "schema export", ok, result, diagnostic.ToMaps(diags)); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return ExitInternal
	}
	writeDiagnosticsText(stderr, diags)
	return code
}
