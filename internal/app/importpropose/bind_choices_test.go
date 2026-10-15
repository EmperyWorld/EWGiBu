package importpropose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyBindChoicesExistingFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "config", "pg.conf")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := map[string]any{
		"spec": map[string]any{
			"resources": map[string]any{
				"res-pg": map[string]any{
					"source":  "data/stack1/postgresql",
					"include": []any{"postgresql.conf"},
				},
			},
		},
	}
	out, diags := ApplyBindChoices(dir, project, []BindChoice{{
		ResourceID: "res-pg",
		Mode:       "existing",
		Path:       "config/pg.conf",
	}})
	if len(diags) > 0 {
		t.Fatalf("diags=%v", diags)
	}
	spec := out["spec"].(map[string]any)
	res := spec["resources"].(map[string]any)["res-pg"].(map[string]any)
	if res["source"] != "config" {
		t.Fatalf("source=%v", res["source"])
	}
}
