package importpropose

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestReconcileResourceOverlapsExcludesNestedFile(t *testing.T) {
	resources := map[string]any{
		"dir": map[string]any{
			"source":  "data/stack1/postgresql",
			"include": []any{"**"},
		},
		"file": map[string]any{
			"source":  "data/stack1/postgresql",
			"include": []any{"postgresql.conf"},
		},
	}
	reconcileResourceOverlaps(resources)
	dir := resources["dir"].(map[string]any)
	exc, _ := dir["exclude"].([]any)
	if len(exc) != 1 || exc[0] != "postgresql.conf" {
		t.Fatalf("exclude=%v", dir["exclude"])
	}
}

func TestEnsureResourceStubsDirsAndFile(t *testing.T) {
	dir := t.TempDir()
	project := map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"res-logs": map[string]any{
					"source":  "logs/stack4/tika",
					"include": []any{"**"},
				},
				"res-script": map[string]any{
					"source":  "scripts",
					"include": []any{"log-to-file.sh"},
				},
			},
		},
	}
	res, diags := EnsureResourceStubs(dir, project)
	if diagnostic.HasError(diags) {
		t.Fatalf("diags=%v", diags)
	}
	if len(res.CreatedDirs) == 0 || len(res.CreatedFiles) != 1 {
		t.Fatalf("result=%+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "logs/stack4/tika")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts/log-to-file.sh")); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureResourceStubsRejectsEnv(t *testing.T) {
	_, diags := EnsureResourceStubs(t.TempDir(), map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"bad": map[string]any{"source": "secrets/.env.prod", "include": []any{"**"}},
			},
		},
	})
	if !diagnostic.HasError(diags) {
		t.Fatal("expected policy error")
	}
}
