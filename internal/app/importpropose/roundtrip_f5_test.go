package importpropose_test

import (
	"path/filepath"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/render/compose"
)

// ADR-020 F5 slice: imported healthcheck argv renders back to Compose CMD-SHELL.
func TestF5HealthcheckRoundTrip(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f1")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "f1",
	})
	if !res.OK {
		t.Fatalf("propose ok=false")
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	tei := services["tei-embed"].(map[string]any)
	hc := tei["healthcheck"].(map[string]any)
	raw := hc["command"].([]any)
	cmd := make([]string, len(raw))
	for i, v := range raw {
		cmd[i], _ = v.(string)
	}
	got := compose.HealthcheckTestFromManifest(cmd)
	if len(got) != 2 || got[0] != "CMD-SHELL" || got[1] == "" {
		t.Fatalf("round-trip healthcheck=%v", got)
	}
}
