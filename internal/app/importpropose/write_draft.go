package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/canonicaljson"
	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
	"iacpack/internal/inputs"
)

// DefaultImportDraftRel is the conventional workspace path for CLI stub commands.
const DefaultImportDraftRel = "imports/import-draft.json"

// WriteDraftResult is the machine output for write-draft.
type WriteDraftResult struct {
	Path string `json:"path"`
}

func (r WriteDraftResult) MapForAPI() map[string]any {
	path := r.Path
	if path == "" {
		path = ""
	}
	return map[string]any{"path": path}
}

// WriteImportDraft writes canonical JSON for a draft Project (explicit authoring).
func WriteImportDraft(workspace, rel string, project map[string]any, replace bool) (written string, diags []diagnostic.Diagnostic) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "workspace is required")}
	}
	if project == nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportParseError, "project is required")}
	}
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		rel = DefaultImportDraftRel
	}
	if err := rejectDraftWritePath(rel); err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPolicyDenied, err.Error())}
	}
	if _, err := path.ParseWorkspacePath(rel); err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "invalid path: "+err.Error())}
	}
	if err := inputs.ValidateRelPath(rel); err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, err.Error())}
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	if !underRoot(workspace, abs) {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportPathInvalid, "path escapes workspace")}
	}
	raw, err := canonicaljson.Marshal(project)
	if err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportParseError, "project: "+err.Error())}
	}
	if _, err := os.Lstat(abs); err == nil && !replace {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeImportCollision,
			fmt.Sprintf("refusing to overwrite %q (pass replace=true)", rel))}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
	}
	if err := os.WriteFile(abs, raw, 0o644); err != nil {
		return "", []diagnostic.Diagnostic{diagnostic.Error(diagnostic.CodeInputIOError, err.Error())}
	}
	return rel, nil
}

func rejectDraftWritePath(rel string) error {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(rel)))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return fmt.Errorf("refusing to write env files")
	}
	if strings.Contains(strings.ToLower(rel), "docker.sock") {
		return fmt.Errorf("forbidden path")
	}
	return nil
}
