package importpropose

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestEnsureBuildContextStubs(t *testing.T) {
	dir := t.TempDir()
	project := map[string]any{
		"spec": map[string]any{
			"images": map[string]any{
				"app": map[string]any{
					"build": map[string]any{
						"context":    "builders/stack4-ocrmypdf",
						"dockerfile": "Dockerfile",
					},
				},
			},
		},
	}
	res, diags := EnsureImportStubs(dir, project)
	if diagnostic.HasError(diags) {
		t.Fatalf("diags=%v", diags)
	}
	if len(res.CreatedBuildContexts) != 1 {
		t.Fatalf("build contexts=%v dirs=%v", res.CreatedBuildContexts, res.CreatedDirs)
	}
	if _, err := os.Stat(filepath.Join(dir, "builders/stack4-ocrmypdf")); err != nil {
		t.Fatal(err)
	}
}
