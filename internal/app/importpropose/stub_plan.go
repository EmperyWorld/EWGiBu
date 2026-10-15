package importpropose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iacpack/internal/diagnostic"
	"iacpack/internal/domain/path"
	"iacpack/internal/inputs"
)

// ImportStubPlan lists workspace paths that EnsureImportStubs would create (dry-run).
type ImportStubPlan struct {
	PlannedDirs          []string
	PlannedFiles         []string
	PlannedBuildContexts []string
	Skipped              []string
	BindItems            []BindPlanItem
}

func (p ImportStubPlan) MapForAPI() map[string]any {
	items := p.BindItems
	if items == nil {
		items = []BindPlanItem{}
	}
	return map[string]any{
		"plannedDirs":          stringSliceOrEmpty(p.PlannedDirs),
		"plannedFiles":         stringSliceOrEmpty(p.PlannedFiles),
		"plannedBuildContexts": stringSliceOrEmpty(p.PlannedBuildContexts),
		"skipped":              stringSliceOrEmpty(p.Skipped),
		"bindItems":            items,
	}
}

// PlanImportStubs computes stub paths without writing the workspace.
func PlanImportStubs(workspace string, project map[string]any) (ImportStubPlan, []diagnostic.Diagnostic) {
	var plan ImportStubPlan
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return plan, []diagnostic.Diagnostic{
			diagnostic.Error(diagnostic.CodeImportPathInvalid, "workspace is required"),
		}
	}
	resPlan, diags := planResourceStubs(workspace, project)
	plan.PlannedDirs = resPlan.PlannedDirs
	plan.PlannedFiles = resPlan.PlannedFiles
	plan.Skipped = resPlan.Skipped
	plan.BindItems = append(plan.BindItems, resPlan.BindItems...)
	builds, buildItems, bd := planBuildContextStubs(workspace, project)
	plan.PlannedBuildContexts = builds
	plan.BindItems = append(plan.BindItems, buildItems...)
	diags = append(diags, bd...)
	sort.Strings(plan.PlannedDirs)
	sort.Strings(plan.PlannedFiles)
	sort.Strings(plan.PlannedBuildContexts)
	sort.Strings(plan.Skipped)
	return plan, diags
}

func planResourceStubs(workspace string, project map[string]any) (ImportStubPlan, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var plan ImportStubPlan
	spec, _ := project["spec"].(map[string]any)
	resources, _ := spec["resources"].(map[string]any)
	if len(resources) == 0 {
		return plan, diags
	}
	ids := make([]string, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	plannedDir := map[string]struct{}{}
	plannedFile := map[string]struct{}{}

	for _, id := range ids {
		m, _ := resources[id].(map[string]any)
		if m == nil {
			continue
		}
		src, _ := m["source"].(string)
		src = filepath.ToSlash(strings.TrimSpace(src))
		if src == "" {
			continue
		}
		ptr := "/spec/resources/" + id + "/source"
		if err := rejectStubPath(src); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPolicyDenied,
				fmt.Sprintf("resource %s: stub refused for %q: %v", id, src, err)).
				WithSource("", ptr, 0, 0))
			continue
		}
		if _, err := path.ParseWorkspacePath(src); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("resource %s: invalid source %q", id, src)).
				WithSource("", ptr, 0, 0))
			continue
		}
		abs := filepath.Join(workspace, filepath.FromSlash(src))
		if !underRoot(workspace, abs) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("resource %s: source escapes workspace", id)).
				WithSource("", ptr, 0, 0))
			continue
		}
		if child := singleFileInclude(m); child != "" {
			childRel := filepath.ToSlash(filepath.Join(src, child))
			childAbs := filepath.Join(workspace, filepath.FromSlash(childRel))
			if _, err := os.Lstat(childAbs); err == nil {
				plan.Skipped = append(plan.Skipped, childRel)
				plan.BindItems = append(plan.BindItems, BindPlanItem{
					ResourceID: id, MissingPath: childRel, Kind: "file",
					Source: src, Include: child, Status: "exists",
				})
				continue
			}
			planPlannedDir(workspace, src, plannedDir, &plan)
			planPlannedFile(childRel, plannedFile, &plan)
			plan.BindItems = append(plan.BindItems, BindPlanItem{
				ResourceID: id, MissingPath: childRel, Kind: "file",
				Source: src, Include: child, Status: "missing",
			})
			continue
		}
		if _, err := os.Lstat(abs); err == nil {
			plan.Skipped = append(plan.Skipped, src)
			plan.BindItems = append(plan.BindItems, BindPlanItem{
				ResourceID: id, MissingPath: src, Kind: "directory",
				Source: src, Status: "exists",
			})
			continue
		}
		planPlannedDir(workspace, src, plannedDir, &plan)
		plan.BindItems = append(plan.BindItems, BindPlanItem{
			ResourceID: id, MissingPath: src, Kind: "directory",
			Source: src, Status: "missing",
		})
	}
	return plan, diags
}

func planPlannedDir(_ string, rel string, seen map[string]struct{}, plan *ImportStubPlan) {
	if rel == "" || rel == "." {
		return
	}
	if _, ok := seen[rel]; ok {
		return
	}
	seen[rel] = struct{}{}
	plan.PlannedDirs = append(plan.PlannedDirs, rel)
}

func planPlannedFile(rel string, seen map[string]struct{}, plan *ImportStubPlan) {
	if rel == "" {
		return
	}
	if _, ok := seen[rel]; ok {
		return
	}
	seen[rel] = struct{}{}
	plan.PlannedFiles = append(plan.PlannedFiles, rel)
}

func planBuildContextStubs(workspace string, project map[string]any) ([]string, []BindPlanItem, []diagnostic.Diagnostic) {
	var diags []diagnostic.Diagnostic
	var planned []string
	var items []BindPlanItem
	spec, _ := project["spec"].(map[string]any)
	images, _ := spec["images"].(map[string]any)
	if len(images) == 0 {
		return planned, items, diags
	}
	seen := map[string]struct{}{}
	for _, id := range sortedImageIDs(images) {
		ctx, ok := imageBuildContext(images[id])
		if !ok {
			continue
		}
		if err := rejectStubPath(ctx); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPolicyDenied,
				fmt.Sprintf("image %s: build context stub refused for %q: %v", id, ctx, err)).
				WithSource("", "/spec/images/"+id+"/build/context", 0, 0))
			continue
		}
		if _, err := path.ParseWorkspacePath(ctx); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("image %s: invalid build context %q", id, ctx)).
				WithSource("", "/spec/images/"+id+"/build/context", 0, 0))
			continue
		}
		abs := filepath.Join(workspace, filepath.FromSlash(ctx))
		if !underRoot(workspace, abs) {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("image %s: build context escapes workspace", id)).
				WithSource("", "/spec/images/"+id+"/build/context", 0, 0))
			continue
		}
		if _, err := os.Lstat(abs); err == nil {
			items = append(items, BindPlanItem{
				ImageID: id, MissingPath: ctx, Kind: "buildContext", Status: "exists",
			})
			continue
		}
		if err := inputs.ValidateRelPath(ctx); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeImportPathInvalid,
				fmt.Sprintf("image %s: invalid build context %q", id, ctx)).
				WithSource("", "/spec/images/"+id+"/build/context", 0, 0))
			continue
		}
		if _, ok := seen[ctx]; ok {
			continue
		}
		seen[ctx] = struct{}{}
		planned = append(planned, ctx)
		items = append(items, BindPlanItem{
			ImageID: id, MissingPath: ctx, Kind: "buildContext", Status: "missing",
		})
	}
	return planned, items, diags
}

func applyImportStubPlan(workspace string, plan ImportStubPlan) (ResourceStubResult, []diagnostic.Diagnostic) {
	var out ResourceStubResult
	var diags []diagnostic.Diagnostic
	createdDir := map[string]struct{}{}
	createdFile := map[string]struct{}{}
	for _, rel := range plan.PlannedDirs {
		if err := mkdirWorkspaceRel(workspace, rel, createdDir, &out); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError,
				fmt.Sprintf("mkdir %q: %v", rel, err)))
		}
	}
	for _, rel := range plan.PlannedFiles {
		if err := writeEmptyStubFile(workspace, rel, createdFile, &out); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError,
				fmt.Sprintf("stub file %q: %v", rel, err)))
		}
	}
	for _, ctx := range plan.PlannedBuildContexts {
		abs := filepath.Join(workspace, filepath.FromSlash(ctx))
		if err := os.MkdirAll(abs, 0o755); err != nil {
			diags = append(diags, diagnostic.Error(diagnostic.CodeInputIOError,
				fmt.Sprintf("mkdir build context %q: %v", ctx, err)))
			continue
		}
		out.CreatedBuildContexts = append(out.CreatedBuildContexts, ctx)
	}
	out.Skipped = append(out.Skipped, plan.Skipped...)
	sort.Strings(out.CreatedDirs)
	sort.Strings(out.CreatedFiles)
	sort.Strings(out.CreatedBuildContexts)
	sort.Strings(out.Skipped)
	return out, diags
}
