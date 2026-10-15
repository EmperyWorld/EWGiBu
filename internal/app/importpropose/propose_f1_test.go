package importpropose_test

import (
	"path/filepath"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func TestProposeF1InverseEmitFixture(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "import", "f1")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "f1",
	})
	for _, d := range res.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected error %s: %s", d.Code, d.Message)
		}
	}
	if !res.OK {
		t.Fatalf("ok=false summary=%+v", res.Summary)
	}
	if res.Summary.Services != 2 {
		t.Fatalf("services=%d", res.Summary.Services)
	}
	spec := res.Project["spec"].(map[string]any)
	images := spec["images"].(map[string]any)
	teiImg := images["tei-embed"].(map[string]any)
	if teiImg["ref"] != "ghcr.io/huggingface/text-embeddings-inference:cpu-1.7.2" {
		t.Fatalf("tei ref=%v", teiImg["ref"])
	}
	services := spec["services"].(map[string]any)
	tei := services["tei-embed"].(map[string]any)
	resMem := tei["resources"].(map[string]any)["memory"]
	if resMem != int64(512<<20) {
		t.Fatalf("tei memory=%v", resMem)
	}
	hc := tei["healthcheck"].(map[string]any)
	cmd := hc["command"].([]any)
	if len(cmd) != 3 || cmd[0] != "/bin/sh" {
		t.Fatalf("healthcheck=%v", cmd)
	}
	worker := services["worker"].(map[string]any)
	wr := worker["resources"].(map[string]any)
	if wr["cpus"] != "2" {
		t.Fatalf("cpus=%v", wr["cpus"])
	}
	if wr["memory"] != int64(256<<20) {
		t.Fatalf("worker memory=%v", wr["memory"])
	}
}
