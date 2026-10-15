package compile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
)

func TestGoldenReleaseGraph(t *testing.T) {
	p := loadProject(t, "examples/release/iacpack.yaml")
	d, err := compile.ToIR(p)
	if err != nil {
		t.Fatal(err)
	}
	g, diags := compile.BuildGraph(d)
	if diagnostic.HasError(diags) {
		t.Fatal(diags)
	}
	text := compile.FormatGraphText(d, g)
	mmd := compile.FormatGraphMermaid(g)
	dir := filepath.Join(repoRoot(t), "testdata", "golden", "graph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(dir, "release.text")
	mmdPath := filepath.Join(dir, "release.mmd")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(textPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mmdPath, []byte(mmd), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantText, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatalf("missing golden %s (run UPDATE_GOLDEN=1): %v", textPath, err)
	}
	wantMMD, err := os.ReadFile(mmdPath)
	if err != nil {
		t.Fatalf("missing golden %s: %v", mmdPath, err)
	}
	if !bytes.Equal([]byte(text), wantText) {
		t.Fatalf("text golden mismatch\n got:\n%s\nwant:\n%s", text, wantText)
	}
	if !bytes.Equal([]byte(mmd), wantMMD) {
		t.Fatalf("mermaid golden mismatch\n got:\n%s\nwant:\n%s", mmd, wantMMD)
	}
}
