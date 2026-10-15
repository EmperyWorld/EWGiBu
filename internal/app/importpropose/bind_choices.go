package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
)

// BindChoice is an operator decision for one bind resource (stub vs use existing workspace path).
type BindChoice struct {
	ResourceID string `json:"resourceId,omitempty"`
	ImageID    string `json:"imageId,omitempty"`
	Mode       string `json:"mode"` // stub | existing
	Path       string `json:"path,omitempty"`
}

// ApplyBindChoices updates spec.resources sources when mode=existing and path exists under workspace.
func ApplyBindChoices(workspace string, project map[string]any, choices []BindChoice) (map[string]any, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	if project == nil || len(choices) == 0 {
		return project, diags
	}
	workspace = strings.TrimSpace(workspace)
	spec, _ := project["spec"].(map[string]any)
	if spec == nil {
		return project, diags
	}
	resources, _ := spec["resources"].(map[string]any)
	if len(resources) == 0 {
		return project, diags
	}
	out := cloneMap(project)
	outSpec := out["spec"].(map[string]any)
	outRes := cloneMap(resources)
	outImages := map[string]any{}
	if raw, ok := spec["images"].(map[string]any); ok {
		outImages = cloneMap(raw)
	}

	for _, ch := range choices {
		if strings.EqualFold(ch.Mode, "stub") || ch.Mode == "" {
			continue
		}
		if !strings.EqualFold(ch.Mode, "existing") {
			id := ch.ResourceID
			if id == "" {
				id = ch.ImageID
			}
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportUnsupportedField,
				fmt.Sprintf("bind choice for %q: unknown mode %q", id, ch.Mode)).
				WithSource("", "/spec/resources/"+ch.ResourceID, 0, 0))
			continue
		}
		rel, ok, d := validateExistingWorkspacePath(workspace, ch.Path)
		if d != nil {
			diags = append(diags, *d)
			continue
		}
		if !ok {
			continue
		}
		if ch.ImageID != "" {
			img, _ := outImages[ch.ImageID].(map[string]any)
			if img == nil {
				diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
					fmt.Sprintf("image %s: not in draft", ch.ImageID)).
					WithSource("", "/spec/images/"+ch.ImageID, 0, 0))
				continue
			}
			build, _ := img["build"].(map[string]any)
			if build == nil {
				build = map[string]any{}
				img["build"] = build
			}
			build["context"] = rel
			outImages[ch.ImageID] = img
			continue
		}
		if ch.ResourceID == "" {
			continue
		}
		entry := resourceEntryForBindSource(rel)
		outRes[ch.ResourceID] = entry
	}
	outSpec["resources"] = outRes
	if len(outImages) > 0 {
		outSpec["images"] = outImages
	}
	return out, diags
}

func validateExistingWorkspacePath(workspace, rawPath string) (rel string, ok bool, d *diagnostic.Diagnostic) {
	rel = filepath.ToSlash(strings.TrimSpace(rawPath))
	if rel == "" {
		return "", false, ptrDiag(diagnostic.Error(diagnostic.CodeImportPathInvalid, "existing workspace path is required"))
	}
	if err := rejectStubPath(rel); err != nil {
		return "", false, ptrDiag(diagnostic.Error(diagnostic.CodeImportPolicyDenied,
			fmt.Sprintf("path refused: %v", err)))
	}
	if _, err := path.ParseWorkspacePath(rel); err != nil {
		return "", false, ptrDiag(diagnostic.Error(diagnostic.CodeImportPathInvalid,
			fmt.Sprintf("invalid path %q", rel)))
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	if !underRoot(workspace, abs) {
		return "", false, ptrDiag(diagnostic.Error(diagnostic.CodeImportPathInvalid, "path escapes workspace"))
	}
	if _, err := os.Lstat(abs); err != nil {
		return "", false, ptrDiag(diagnostic.Warning(diagnostic.CodeImportPathInvalid,
			fmt.Sprintf("path %q not found in workspace yet", rel)))
	}
	return rel, true, nil
}

func ptrDiag(d diagnostic.Diagnostic) *diagnostic.Diagnostic {
	return &d
}
