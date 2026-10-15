package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/inputs"
)

// ResourceStubResult lists workspace paths created by EnsureResourceStubs.
type ResourceStubResult struct {
	CreatedDirs          []string `json:"createdDirs"`
	CreatedFiles         []string `json:"createdFiles"`
	CreatedBuildContexts []string `json:"createdBuildContexts"`
	Skipped              []string `json:"skipped"`
}

// MapForAPI returns a JSON-friendly result map with empty slices instead of null.
func (r ResourceStubResult) MapForAPI() map[string]any {
	return map[string]any{
		"createdDirs":          stringSliceOrEmpty(r.CreatedDirs),
		"createdFiles":         stringSliceOrEmpty(r.CreatedFiles),
		"createdBuildContexts": stringSliceOrEmpty(r.CreatedBuildContexts),
		"skipped":              stringSliceOrEmpty(r.Skipped),
	}
}

func stringSliceOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// EnsureImportStubs creates missing bind-resource paths and image build context directories.
func EnsureImportStubs(workspace string, project map[string]any) (ResourceStubResult, []diagnostic.Diagnostic) {
	plan, diags := PlanImportStubs(workspace, project)
	res, applyDiags := applyImportStubPlan(workspace, plan)
	diags = append(diags, applyDiags...)
	return res, diags
}

// EnsureResourceStubs is an alias for EnsureImportStubs (bind resources + build contexts).
func EnsureResourceStubs(workspace string, project map[string]any) (ResourceStubResult, []diagnostic.Diagnostic) {
	return EnsureImportStubs(workspace, project)
}

// reconcileResourceOverlaps excludes single-file binds from a sibling directory
// resource that uses include **, so the same workspace file is not packaged twice.
func reconcileResourceOverlaps(resources map[string]any) {
	ids := make([]string, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, dirID := range ids {
		dir, _ := resources[dirID].(map[string]any)
		if dir == nil || !includeIsAll(dir) {
			continue
		}
		src, _ := dir["source"].(string)
		var extra []string
		for _, fileID := range ids {
			if fileID == dirID {
				continue
			}
			file, _ := resources[fileID].(map[string]any)
			if file == nil {
				continue
			}
			fsrc, _ := file["source"].(string)
			if fsrc != src {
				continue
			}
			pat := singleConcreteInclude(file)
			if pat == "" {
				continue
			}
			extra = append(extra, pat)
		}
		if len(extra) == 0 {
			continue
		}
		sort.Strings(extra)
		existing := anyStringList(dir["exclude"])
		seen := map[string]struct{}{}
		var merged []any
		for _, s := range append(existing, extra...) {
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			merged = append(merged, s)
		}
		dir["exclude"] = merged
	}
}

func includeIsAll(m map[string]any) bool {
	inc := anyStringList(m["include"])
	return len(inc) == 1 && inc[0] == "**"
}

func singleConcreteInclude(m map[string]any) string {
	inc := anyStringList(m["include"])
	if len(inc) != 1 || inc[0] == "**" || strings.ContainsAny(inc[0], "*?[") {
		return ""
	}
	return inc[0]
}

func anyStringList(raw any) []string {
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			s, _ := item.(string)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

func resourceEntryForBindSource(resolved string) map[string]any {
	source, include := resourceDeclFromBindSource(resolved)
	entry := map[string]any{"source": source, "include": include}
	if len(include) == 1 {
		if pat, _ := include[0].(string); pat == "**" {
			entry["allowEmpty"] = true
		}
	}
	return entry
}

func rejectStubPath(rel string) error {
	low := strings.ToLower(rel)
	if strings.Contains(low, "docker.sock") {
		return fmt.Errorf("docker.sock path forbidden")
	}
	base := strings.ToLower(filepath.Base(filepath.FromSlash(rel)))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return fmt.Errorf(".env paths forbidden")
	}
	if err := inputs.ValidateRelPath(rel); err != nil {
		return err
	}
	return nil
}

func resourceDeclFromBindSource(resolved string) (source string, include []any) {
	resolved = filepath.ToSlash(strings.TrimSpace(resolved))
	if !resourceSourceLooksLikeFile(resolved) {
		return resolved, []any{"**"}
	}
	dir := filepath.ToSlash(filepath.Dir(resolved))
	if dir == "." {
		dir = ""
	}
	base := filepath.Base(resolved)
	if dir == "" {
		return base, []any{base}
	}
	return dir, []any{base}
}

func resourceSourceLooksLikeFile(rel string) bool {
	base := filepath.Base(filepath.FromSlash(rel))
	if base == "" || base == "." || base == ".." {
		return false
	}
	if strings.HasPrefix(base, ".") {
		return false
	}
	dot := strings.LastIndex(base, ".")
	if dot <= 0 {
		return false
	}
	ext := strings.ToLower(base[dot+1:])
	switch ext {
	case "sh", "bash", "py", "pl", "rb", "js", "ts", "json", "yaml", "yml", "txt", "md", "conf", "cfg", "ini":
		return true
	default:
		return len(ext) >= 1 && len(ext) <= 8
	}
}

func mkdirWorkspaceRel(workspace, rel string, seen map[string]struct{}, out *ResourceStubResult) error {
	if rel == "" || rel == "." {
		return nil
	}
	if _, ok := seen[rel]; ok {
		return nil
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	seen[rel] = struct{}{}
	out.CreatedDirs = append(out.CreatedDirs, rel)
	return nil
}

func writeEmptyStubFile(workspace, rel string, seen map[string]struct{}, out *ResourceStubResult) error {
	if _, ok := seen[rel]; ok {
		return nil
	}
	abs := filepath.Join(workspace, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, nil, 0o644); err != nil {
		return err
	}
	seen[rel] = struct{}{}
	out.CreatedFiles = append(out.CreatedFiles, rel)
	return nil
}
