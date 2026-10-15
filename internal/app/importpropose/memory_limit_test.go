package importpropose

import (
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
)

func TestComposeMemoryBytesFromNodeWithDefault(t *testing.T) {
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  app:
    mem_limit: ${MEM:-8G}
`))
	n := doc.Root.GetMap("services").GetMap("app").GetMap("mem_limit")
	b, ok, diags := composeMemoryBytesFromNode(n)
	if !ok || b != 8<<30 {
		t.Fatalf("bytes=%d ok=%v diags=%v", b, ok, diags)
	}
}

func TestApplyServiceMemoryLimitNoConflictWithDeploy(t *testing.T) {
	svc := map[string]any{
		"resources": map[string]any{"memory": int64(1 << 30)},
	}
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  app:
    mem_limit: 2G
`))
	n := doc.Root.GetMap("services").GetMap("app").GetMap("mem_limit")
	var diags []diagnostic.Diagnostic
	applyServiceMemoryLimit(svc, "app", "f.yml", "/services/app/mem_limit", n, &diags)
	if len(diags) != 1 || diags[0].Severity != diagnostic.SeverityError {
		t.Fatalf("expected conflict error, diags=%v", diags)
	}
}
