package importpropose_test

import (
	"path/filepath"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func TestProposeF3DeployGPU(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f3")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "f3",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	if res.Summary.Services != 2 {
		t.Fatalf("services=%d want llama and wide", res.Summary.Services)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	llama := services["llama"].(map[string]any)
	limits := llama["resources"].(map[string]any)
	if limits["gpu"] != 1 {
		t.Fatalf("gpu=%v", limits["gpu"])
	}
	wide := services["wide"].(map[string]any)
	if wide["resources"].(map[string]any)["gpu"] != "all" {
		t.Fatalf("wide gpu=%v", wide["resources"])
	}
	if _, ok := services["cdi"]; ok {
		t.Fatal("cdi service must stay excluded")
	}
	var policy int
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportPolicyDenied && d.Severity == diagnostic.SeverityWarning {
			policy++
		}
	}
	if policy < 1 {
		t.Fatalf("expected devices policy warning, diags=%v", res.Diagnostics)
	}
}
