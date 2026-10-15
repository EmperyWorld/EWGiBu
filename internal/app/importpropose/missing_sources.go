package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iacpack/internal/diagnostic"
)

// MissingResourceSourceDiagsForAPI returns path warnings for a draft project (HTTP/UI).
func MissingResourceSourceDiagsForAPI(workspace string, project map[string]any) []diagnostic.Diagnostic {
	spec, _ := project["spec"].(map[string]any)
	resources, _ := spec["resources"].(map[string]any)
	return missingResourceSourceDiags(workspace, resources)
}

func missingResourceSourceDiags(workspace string, resources map[string]any) []diagnostic.Diagnostic {
	if len(resources) == 0 || strings.TrimSpace(workspace) == "" {
		return nil
	}
	var out []diagnostic.Diagnostic
	for id, raw := range resources {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		missing, rel := resourceBindMissing(workspace, m)
		if !missing {
			continue
		}
		out = append(out, diagnostic.Warning(diagnostic.CodeImportPathInvalid,
			fmt.Sprintf("resource %s: bind source %q not found in workspace (create before validate/build)", id, rel)).
			WithSource("", "/spec/resources/"+id+"/source", 0, 0).
			WithHint("Use Create stub directories in the import panel or add paths under the workspace"))
	}
	return out
}

func resourceBindMissing(workspace string, m map[string]any) (missing bool, relPath string) {
	src, _ := m["source"].(string)
	src = filepath.ToSlash(strings.TrimSpace(src))
	if src == "" {
		return false, ""
	}
	abs := filepath.Join(workspace, filepath.FromSlash(src))
	info, err := os.Lstat(abs)
	if err != nil {
		return true, src
	}
	if !info.IsDir() {
		return true, src
	}
	if child := singleFileInclude(m); child != "" {
		childAbs := filepath.Join(abs, filepath.FromSlash(child))
		if _, err := os.Lstat(childAbs); err != nil {
			return true, filepath.ToSlash(filepath.Join(src, child))
		}
	}
	return false, ""
}

func singleFileInclude(m map[string]any) string {
	raw, _ := m["include"].([]any)
	if len(raw) != 1 {
		return ""
	}
	pat, _ := raw[0].(string)
	pat = strings.TrimSpace(pat)
	if pat == "" || pat == "**" || strings.ContainsAny(pat, "*?[") {
		return ""
	}
	return pat
}
