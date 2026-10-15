package discover_test

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/workspace"
	"iacpack/internal/discover"
)

func TestDiscoverDepthAndSorting(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Dockerfile"), "FROM a\n")
	mustWrite(t, filepath.Join(dir, "compose.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(dir, "compose.site.yml"), "services: {}\n")
	mustWrite(t, filepath.Join(dir, "stack9.yml"), "services: {}\n")
	mustWrite(t, filepath.Join(dir, "docker-compose.prod.yaml"), "services: {}\n")
	mustWrite(t, filepath.Join(dir, "nested", "deep", "too", "far", "Dockerfile"), "FROM b\n") // beyond depth 3
	mustWrite(t, filepath.Join(dir, "svc", "Dockerfile.app"), "FROM c\n")
	mustWrite(t, filepath.Join(dir, "dist", "Dockerfile"), "FROM d\n") // output root skipped
	mustWrite(t, filepath.Join(dir, ".env"), "x=1\n")

	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sugs, err := discover.Run(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) < 5 {
		t.Fatalf("sugs=%+v", sugs)
	}
	for i := 1; i < len(sugs); i++ {
		if sugs[i-1].Path > sugs[i].Path {
			t.Fatalf("unsorted: %+v", sugs)
		}
	}
	want := map[string]string{
		"Dockerfile":               "dockerfile",
		"compose.yaml":             "compose",
		"compose.site.yml":         "compose",
		"stack9.yml":               "compose",
		"docker-compose.prod.yaml": "compose",
		"svc/Dockerfile.app":       "dockerfile",
	}
	got := map[string]string{}
	for _, s := range sugs {
		got[s.Path] = s.Kind
		if s.Path == ".env" || s.Path == "dist/Dockerfile" || s.Path == "nested/deep/too/far/Dockerfile" {
			t.Fatalf("unexpected suggestion %q", s.Path)
		}
	}
	for p, k := range want {
		if got[p] != k {
			t.Fatalf("path %s: kind=%q want %q; all=%+v", p, got[p], k, sugs)
		}
	}
}

func TestClassifyKind(t *testing.T) {
	cases := map[string]string{
		"compose.yml":                 "compose",
		"stack0_b.yml":                "compose",
		"stack1.YAML":                 "compose",
		"readme.md":                   "",
		"docker-compose.override.yml": "compose",
	}
	for name, want := range cases {
		if got := discover.ClassifyKind(name); got != want {
			t.Fatalf("%s: got %q want %q", name, got, want)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
