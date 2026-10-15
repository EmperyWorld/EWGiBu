package importpropose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestWriteImportDraftCreatesAndReplace(t *testing.T) {
	dir := t.TempDir()
	project := map[string]any{"apiVersion": "iacpack.io/v1alpha1", "kind": "Project", "spec": map[string]any{}}
	rel, diags := WriteImportDraft(dir, DefaultImportDraftRel, project, false)
	if diagnostic.HasError(diags) || rel != DefaultImportDraftRel {
		t.Fatalf("rel=%q diags=%v", rel, diags)
	}
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if _, err := os.Stat(abs); err != nil {
		t.Fatal(err)
	}
	_, diags = WriteImportDraft(dir, DefaultImportDraftRel, project, false)
	if !diagnostic.HasError(diags) {
		t.Fatal("expected collision without replace")
	}
	rel, diags = WriteImportDraft(dir, DefaultImportDraftRel, map[string]any{
		"apiVersion": "iacpack.io/v1alpha1",
		"kind":       "Project",
		"spec":       map[string]any{"services": map[string]any{}},
	}, true)
	if diagnostic.HasError(diags) {
		t.Fatalf("replace: %v", diags)
	}
	raw, _ := os.ReadFile(abs)
	body := string(raw)
	if !strings.Contains(body, `"kind"`) || !strings.Contains(body, `"services"`) {
		t.Fatalf("body %s", raw)
	}
}
