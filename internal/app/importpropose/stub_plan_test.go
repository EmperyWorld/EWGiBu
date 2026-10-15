package importpropose

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestPlanImportStubsMatchesEnsure(t *testing.T) {
	dir := t.TempDir()
	project := map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"res-logs": map[string]any{
					"source":  "logs/stack4/tika",
					"include": []any{"**"},
				},
			},
			"images": map[string]any{
				"ocrmypdf-api": map[string]any{
					"build": map[string]any{
						"context":    "builders/stack4-ocrmypdf",
						"dockerfile": "Dockerfile",
						"include":    []any{"**"},
					},
				},
			},
		},
	}
	plan, planDiags := PlanImportStubs(dir, project)
	if diagnostic.HasError(planDiags) {
		t.Fatalf("plan diags: %v", planDiags)
	}
	if len(plan.PlannedDirs) == 0 || len(plan.PlannedBuildContexts) == 0 {
		t.Fatalf("plan=%+v", plan)
	}
	got, ensureDiags := EnsureImportStubs(dir, project)
	if diagnostic.HasError(ensureDiags) {
		t.Fatalf("ensure diags: %v", ensureDiags)
	}
	if !sameStringSet(plan.PlannedDirs, got.CreatedDirs) {
		t.Fatalf("dirs plan=%v got=%v", plan.PlannedDirs, got.CreatedDirs)
	}
	if !sameStringSet(plan.PlannedBuildContexts, got.CreatedBuildContexts) {
		t.Fatalf("build plan=%v got=%v", plan.PlannedBuildContexts, got.CreatedBuildContexts)
	}
	if _, err := os.Stat(filepath.Join(dir, "builders/stack4-ocrmypdf")); err != nil {
		t.Fatal(err)
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]struct{}{}
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			return false
		}
	}
	return true
}
