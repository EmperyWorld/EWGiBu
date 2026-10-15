package importpropose

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestPrepareImportWorkspace(t *testing.T) {
	dir := t.TempDir()
	project := map[string]any{
		"apiVersion": "iacpack.io/v1alpha1",
		"kind":       "Project",
		"spec": map[string]any{
			"resources": map[string]any{
				"res-logs": map[string]any{
					"source":  "logs/prepare",
					"include": []any{"**"},
				},
			},
		},
	}
	res, diags := PrepareImportWorkspace(dir, project, DefaultImportDraftRel, true)
	if diagnostic.HasError(diags) {
		t.Fatalf("diags=%v", diags)
	}
	if res.DraftPath != DefaultImportDraftRel {
		t.Fatalf("path=%q", res.DraftPath)
	}
	if _, err := os.Stat(filepath.Join(dir, "imports", "import-draft.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "logs/prepare")); err != nil {
		t.Fatal(err)
	}
	if len(res.Stubs.CreatedDirs) == 0 {
		t.Fatalf("stubs=%+v", res.Stubs)
	}
}
