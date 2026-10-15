package importpropose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func TestProposeReportsMigrateGap(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("compose.yaml", `
services:
  postgres:
    image: postgres:16
    healthcheck:
      test: ["CMD", "pg_isready"]
  api:
    image: api:dev
`)
	mustWrite("shared/api/migrations/pending/001_init.sql", "SELECT 1;\n")
	// no baseline.sql → gap hint mentions missing baseline

	res := importpropose.Propose(importpropose.Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	var saw bool
	for _, d := range res.Diagnostics {
		if d.Code != diagnostic.CodeImportMigrateGap {
			continue
		}
		saw = true
		if !strings.Contains(d.Message, "shared/api/migrations") {
			t.Fatalf("message=%s", d.Message)
		}
		if d.Hint == "" || !strings.Contains(d.Hint, "baseline.sql") || !strings.Contains(d.Hint, "completedSuccessfully") {
			t.Fatalf("hint=%s", d.Hint)
		}
	}
	if !saw {
		t.Fatalf("diags=%v", res.Diagnostics)
	}
	var refusal bool
	for _, r := range res.Refusals {
		if r.Code == diagnostic.CodeImportMigrateGap {
			refusal = true
		}
	}
	if !refusal {
		t.Fatalf("refusals=%v", res.Refusals)
	}
}
