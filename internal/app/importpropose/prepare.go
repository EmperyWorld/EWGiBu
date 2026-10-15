package importpropose

import "iacpack/internal/diagnostic"

// PrepareResult is the outcome of writing a draft JSON file and creating workspace stubs.
type PrepareResult struct {
	DraftPath string
	Stubs     ResourceStubResult
}

func (r PrepareResult) MapForAPI() map[string]any {
	out := map[string]any{
		"draftPath": r.DraftPath,
		"stubs":     r.Stubs.MapForAPI(),
	}
	return out
}

// PrepareImportWorkspace writes canonical draft JSON then creates bind/build stubs.
func PrepareImportWorkspace(workspace string, project map[string]any, draftPath string, replace bool) (PrepareResult, []diagnostic.Diagnostic) {
	var out PrepareResult
	written, diags := WriteImportDraft(workspace, draftPath, project, replace)
	if diagnostic.HasError(diags) {
		return out, diags
	}
	out.DraftPath = written
	stubs, stubDiags := EnsureImportStubs(workspace, project)
	out.Stubs = stubs
	diags = append(diags, stubDiags...)
	return out, diags
}
