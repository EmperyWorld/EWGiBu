package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func runImport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printImportUsage(stderr)
		return ExitUsage
	}
	switch args[0] {
	case "propose":
		return runImportPropose(args[1:], stdout, stderr)
	case "resource-stubs":
		return runImportResourceStubs(args[1:], stdout, stderr)
	case "stub-preview":
		return runImportStubPreview(args[1:], stdout, stderr)
	case "write-draft":
		return runImportWriteDraft(args[1:], stdout, stderr)
	case "prepare":
		return runImportPrepare(args[1:], stdout, stderr)
	case "-h", "--help":
		printImportUsage(stdout)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown import subcommand %q\n", args[0])
		return ExitUsage
	}
}

func printImportUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  iacpack import propose --workspace <path> [--compose <rel> ...] [--dockerfile <rel> ...] [--profile <name> ...] [--import-strict] [--name <id>] [--output text|json]")
	fmt.Fprintln(w, "  iacpack import resource-stubs --workspace <path> --project-file <json> [--output text|json]")
	fmt.Fprintln(w, "  iacpack import stub-preview --workspace <path> --project-file <json> [--output text|json]")
	fmt.Fprintln(w, "  iacpack import write-draft --workspace <path> --project-file <json> [--path <rel>] [--replace] [--output text|json]")
	fmt.Fprintln(w, "  propose builds a reviewable Project draft (never writes workspace).")
	fmt.Fprintln(w, "  resource-stubs creates empty bind/build directories for a draft Project JSON file.")
	fmt.Fprintln(w, "  stub-preview lists the same paths without writing the workspace.")
	fmt.Fprintln(w, "  write-draft writes canonical draft Project JSON into the workspace (default imports/import-draft.json).")
	fmt.Fprintln(w, "  iacpack import prepare --workspace <path> --project-file <json> [--path <rel>] [--replace] [--output text|json]")
	fmt.Fprintln(w, "  prepare runs write-draft then resource-stubs for the same project file.")
}

func runImportPropose(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	workspacePath := ""
	name := ""
	var composes []string
	var dockerfiles []string
	var activeProfiles []string
	strict := false
	existingFile := ""

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintln(stdout, "usage: iacpack import propose --workspace <path> [--compose <rel> ...] [--dockerfile <rel> ...] [--profile <name> ...] [--import-strict] [--name <id>] [--existing-project-file <json>] [--output text|json]")
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--workspace requires a value")},
				}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--existing-project-file":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--existing-project-file requires a value")},
				}, ExitUsage)
			}
			existingFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--existing-project-file="):
			existingFile = strings.TrimPrefix(a, "--existing-project-file=")
		case a == "--compose":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--compose requires a value")},
				}, ExitUsage)
			}
			composes = append(composes, args[i+1])
			i++
		case strings.HasPrefix(a, "--compose="):
			composes = append(composes, strings.TrimPrefix(a, "--compose="))
		case a == "--dockerfile":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--dockerfile requires a value")},
				}, ExitUsage)
			}
			dockerfiles = append(dockerfiles, args[i+1])
			i++
		case strings.HasPrefix(a, "--dockerfile="):
			dockerfiles = append(dockerfiles, strings.TrimPrefix(a, "--dockerfile="))
		case a == "--name":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--name requires a value")},
				}, ExitUsage)
			}
			name = args[i+1]
			i++
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case a == "--profile":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--profile requires a value")},
				}, ExitUsage)
			}
			activeProfiles = append(activeProfiles, splitProfileCLIArg(args[i+1])...)
			i++
		case strings.HasPrefix(a, "--profile="):
			activeProfiles = append(activeProfiles, splitProfileCLIArg(strings.TrimPrefix(a, "--profile="))...)
		case a == "--import-strict":
			strict = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
					Diagnostics: []diagnostic.Diagnostic{usageDiag("--output requires a value")},
				}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
				Diagnostics: []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))},
			}, ExitUsage)
		}
	}

	if outputMode != "text" && outputMode != "json" {
		return emitImportPropose(stdout, stderr, "text", importpropose.Result{
			Diagnostics: []diagnostic.Diagnostic{usageDiag("--output must be text or json")},
		}, ExitUsage)
	}
	if workspacePath == "" {
		return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{
			Diagnostics: []diagnostic.Diagnostic{usageDiag("import propose requires --workspace <path>")},
		}, ExitUsage)
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		d := diagnostic.Error(diagnostic.CodeInputIOError, "workspace: "+err.Error())
		return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{Diagnostics: []diagnostic.Diagnostic{d}}, ExitIO)
	}
	var existing map[string]any
	if existingFile != "" {
		raw, err := os.ReadFile(existingFile)
		if err != nil {
			d := diagnostic.Error(diagnostic.CodeInputIOError, "existing project: "+err.Error())
			return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{Diagnostics: []diagnostic.Diagnostic{d}}, ExitIO)
		}
		if err := json.Unmarshal(raw, &existing); err != nil {
			d := diagnostic.Error(diagnostic.CodeManifestParseError, "existing project: "+err.Error())
			return emitImportPropose(stdout, stderr, outputMode, importpropose.Result{Diagnostics: []diagnostic.Diagnostic{d}}, ExitInvalid)
		}
	}

	res := importpropose.Propose(importpropose.Request{
		Workspace:       abs,
		Composes:        composes,
		Dockerfiles:     dockerfiles,
		Name:            name,
		ActiveProfiles:  activeProfiles,
		Strict:          strict,
		ExistingProject: existing,
	})
	code := ExitOK
	if !res.OK {
		code = mapExitFromDiags(res.Diagnostics, ExitInvalid)
	}
	return emitImportPropose(stdout, stderr, outputMode, res, code)
}

func runImportStubPreview(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	workspacePath := ""
	projectFile := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printImportUsage(stdout)
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--project-file":
			if i+1 >= len(args) {
				return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag("--project-file requires a value")}, ExitUsage)
			}
			projectFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--project-file="):
			projectFile = strings.TrimPrefix(a, "--project-file=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	if outputMode != "text" && outputMode != "json" {
		return emitImportStubPreview(stdout, stderr, "text", importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag("--output must be text or json")}, ExitUsage)
	}
	if workspacePath == "" || projectFile == "" {
		return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, []diagnostic.Diagnostic{usageDiag("stub-preview requires --workspace and --project-file")}, ExitUsage)
	}
	abs, project, diags, code := loadImportProjectFile(workspacePath, projectFile)
	if code != ExitOK {
		return emitImportStubPreview(stdout, stderr, outputMode, importpropose.ImportStubPlan{}, diags, code)
	}
	plan, planDiags := importpropose.PlanImportStubs(abs, project)
	diags = append(diags, planDiags...)
	exit := ExitOK
	if diagnostic.HasError(diags) {
		exit = mapExitFromDiags(diags, ExitInvalid)
	}
	return emitImportStubPreview(stdout, stderr, outputMode, plan, diags, exit)
}

func runImportPrepare(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	workspacePath := ""
	projectFile := ""
	destPath := ""
	replace := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printImportUsage(stdout)
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--project-file":
			if i+1 >= len(args) {
				return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("--project-file requires a value")}, ExitUsage)
			}
			projectFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--project-file="):
			projectFile = strings.TrimPrefix(a, "--project-file=")
		case a == "--path":
			if i+1 >= len(args) {
				return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("--path requires a value")}, ExitUsage)
			}
			destPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--path="):
			destPath = strings.TrimPrefix(a, "--path=")
		case a == "--replace":
			replace = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	if outputMode != "text" && outputMode != "json" {
		return emitImportPrepare(stdout, stderr, "text", importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("--output must be text or json")}, ExitUsage)
	}
	if workspacePath == "" || projectFile == "" {
		return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, []diagnostic.Diagnostic{usageDiag("prepare requires --workspace and --project-file")}, ExitUsage)
	}
	abs, project, diags, code := loadImportProjectFile(workspacePath, projectFile)
	if code != ExitOK {
		return emitImportPrepare(stdout, stderr, outputMode, importpropose.PrepareResult{}, diags, code)
	}
	res, prepDiags := importpropose.PrepareImportWorkspace(abs, project, destPath, replace)
	diags = append(diags, prepDiags...)
	if diagnostic.HasError(diags) {
		code = mapExitFromDiags(diags, ExitInvalid)
	} else {
		code = ExitOK
	}
	return emitImportPrepare(stdout, stderr, outputMode, res, diags, code)
}

func runImportWriteDraft(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	workspacePath := ""
	projectFile := ""
	destPath := ""
	replace := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printImportUsage(stdout)
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--project-file":
			if i+1 >= len(args) {
				return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("--project-file requires a value")}, ExitUsage)
			}
			projectFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--project-file="):
			projectFile = strings.TrimPrefix(a, "--project-file=")
		case a == "--path":
			if i+1 >= len(args) {
				return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("--path requires a value")}, ExitUsage)
			}
			destPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--path="):
			destPath = strings.TrimPrefix(a, "--path=")
		case a == "--replace":
			replace = true
		case a == "--output":
			if i+1 >= len(args) {
				return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	if outputMode != "text" && outputMode != "json" {
		return emitImportWriteDraft(stdout, stderr, "text", importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("--output must be text or json")}, ExitUsage)
	}
	if workspacePath == "" || projectFile == "" {
		return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, []diagnostic.Diagnostic{usageDiag("write-draft requires --workspace and --project-file")}, ExitUsage)
	}
	abs, project, diags, code := loadImportProjectFile(workspacePath, projectFile)
	if code != ExitOK {
		return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{}, diags, code)
	}
	written, writeDiags := importpropose.WriteImportDraft(abs, destPath, project, replace)
	diags = append(diags, writeDiags...)
	if diagnostic.HasError(diags) {
		code = mapExitFromDiags(diags, ExitInvalid)
	} else {
		code = ExitOK
	}
	return emitImportWriteDraft(stdout, stderr, outputMode, importpropose.WriteDraftResult{Path: written}, diags, code)
}

func runImportResourceStubs(args []string, stdout, stderr io.Writer) int {
	outputMode := "text"
	workspacePath := ""
	projectFile := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			printImportUsage(stdout)
			return ExitOK
		case a == "--workspace":
			if i+1 >= len(args) {
				return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag("--workspace requires a value")}, ExitUsage)
			}
			workspacePath = args[i+1]
			i++
		case strings.HasPrefix(a, "--workspace="):
			workspacePath = strings.TrimPrefix(a, "--workspace=")
		case a == "--project-file":
			if i+1 >= len(args) {
				return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag("--project-file requires a value")}, ExitUsage)
			}
			projectFile = args[i+1]
			i++
		case strings.HasPrefix(a, "--project-file="):
			projectFile = strings.TrimPrefix(a, "--project-file=")
		case a == "--output":
			if i+1 >= len(args) {
				return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag("--output requires a value")}, ExitUsage)
			}
			outputMode = strings.ToLower(args[i+1])
			i++
		case strings.HasPrefix(a, "--output="):
			outputMode = strings.ToLower(strings.TrimPrefix(a, "--output="))
		default:
			return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag(fmt.Sprintf("unexpected argument %q", a))}, ExitUsage)
		}
	}
	if outputMode != "text" && outputMode != "json" {
		return emitImportResourceStubs(stdout, stderr, "text", importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag("--output must be text or json")}, ExitUsage)
	}
	if workspacePath == "" || projectFile == "" {
		return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, []diagnostic.Diagnostic{usageDiag("resource-stubs requires --workspace and --project-file")}, ExitUsage)
	}
	abs, project, diags, code := loadImportProjectFile(workspacePath, projectFile)
	if code != ExitOK {
		return emitImportResourceStubs(stdout, stderr, outputMode, importpropose.ResourceStubResult{}, diags, code)
	}
	stubs, stubDiags := importpropose.EnsureImportStubs(abs, project)
	diags = append(diags, stubDiags...)
	if diagnostic.HasError(diags) {
		code = mapExitFromDiags(diags, ExitInvalid)
	} else {
		code = ExitOK
	}
	return emitImportResourceStubs(stdout, stderr, outputMode, stubs, diags, code)
}

func loadImportProjectFile(workspacePath, projectFile string) (abs string, project map[string]any, diags []diagnostic.Diagnostic, code int) {
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "workspace: "+err.Error())}, ExitIO
	}
	raw, err := os.ReadFile(projectFile)
	if err != nil {
		return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, "project-file: "+err.Error())}, ExitIO
	}
	if err := json.Unmarshal(raw, &project); err != nil {
		return "", nil, []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportParseError, "project-file: "+err.Error())}, ExitInvalid
	}
	return abs, project, nil, ExitOK
}

func emitImportPrepare(stdout, stderr io.Writer, mode string, res importpropose.PrepareResult, diags []diagnostic.Diagnostic, code int) int {
	ok := !diagnostic.HasError(diags)
	result := res.MapForAPI()
	if mode == "json" {
		if err := WriteJSON(stdout, "import prepare", ok, result, diagnostic.ToMaps(diags)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, diags)
	if ok {
		fmt.Fprintf(stdout, "ok: draft=%q created dirs=%d files=%d buildContexts=%d\n",
			res.DraftPath, len(res.Stubs.CreatedDirs), len(res.Stubs.CreatedFiles), len(res.Stubs.CreatedBuildContexts))
	} else {
		fmt.Fprintln(stdout, "import prepare failed")
	}
	return code
}

func emitImportWriteDraft(stdout, stderr io.Writer, mode string, res importpropose.WriteDraftResult, diags []diagnostic.Diagnostic, code int) int {
	ok := !diagnostic.HasError(diags)
	result := res.MapForAPI()
	if mode == "json" {
		if err := WriteJSON(stdout, "import write-draft", ok, result, diagnostic.ToMaps(diags)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, diags)
	if ok {
		fmt.Fprintf(stdout, "ok: wrote draft %q\n", res.Path)
	} else {
		fmt.Fprintln(stdout, "import write-draft failed")
	}
	return code
}

func emitImportStubPreview(stdout, stderr io.Writer, mode string, plan importpropose.ImportStubPlan, diags []diagnostic.Diagnostic, code int) int {
	ok := !diagnostic.HasError(diags)
	result := plan.MapForAPI()
	if mode == "json" {
		if err := WriteJSON(stdout, "import stub-preview", ok, result, diagnostic.ToMaps(diags)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, diags)
	if ok {
		fmt.Fprintf(stdout, "ok: planned dirs=%d files=%d buildContexts=%d skipped=%d\n",
			len(plan.PlannedDirs), len(plan.PlannedFiles), len(plan.PlannedBuildContexts), len(plan.Skipped))
	} else {
		fmt.Fprintln(stdout, "import stub-preview failed")
	}
	return code
}

func emitImportResourceStubs(stdout, stderr io.Writer, mode string, stubs importpropose.ResourceStubResult, diags []diagnostic.Diagnostic, code int) int {
	ok := !diagnostic.HasError(diags)
	result := stubs.MapForAPI()
	if mode == "json" {
		if err := WriteJSON(stdout, "import resource-stubs", ok, result, diagnostic.ToMaps(diags)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, diags)
	if ok {
		fmt.Fprintf(stdout, "ok: created dirs=%d files=%d buildContexts=%d skipped=%d\n",
			len(stubs.CreatedDirs), len(stubs.CreatedFiles), len(stubs.CreatedBuildContexts), len(stubs.Skipped))
	} else {
		fmt.Fprintln(stdout, "import resource-stubs failed")
	}
	return code
}

func splitProfileCLIArg(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func emitImportPropose(stdout, stderr io.Writer, mode string, res importpropose.Result, code int) int {
	refusals := res.Refusals
	if refusals == nil {
		refusals = []importpropose.Refusal{}
	}
	gaps := res.ParameterGaps
	if gaps == nil {
		gaps = []importpropose.ParameterGap{}
	}
	sugs := res.SecretSuggestions
	if sugs == nil {
		sugs = []importpropose.SecretSuggestion{}
	}
	result := map[string]any{
		"project":           res.Project,
		"summary":           res.Summary,
		"sources":           res.Sources,
		"refusals":          refusals,
		"parameterGaps":     gaps,
		"secretSuggestions": sugs,
		"envExample":        res.EnvExample,
	}
	if res.Project == nil {
		result["project"] = nil
	}
	if res.Sources == nil {
		result["sources"] = map[string]any{}
	}
	if mode == "json" {
		if err := WriteJSON(stdout, "import propose", res.OK, result, diagnostic.ToMaps(res.Diagnostics)); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return ExitInternal
		}
		return code
	}
	writeDiagnosticsText(stderr, res.Diagnostics)
	if res.OK {
		fmt.Fprintf(stdout, "ok: draft services=%d images=%d networks=%d volumes=%d parameters=%d refusals=%d gaps=%d secrets=%d\n",
			res.Summary.Services, res.Summary.Images, res.Summary.Networks, res.Summary.Volumes, res.Summary.Parameters,
			len(refusals), len(gaps), len(sugs))
	} else {
		fmt.Fprintln(stdout, "import propose failed")
	}
	return code
}
