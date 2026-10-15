package lockfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/adapters/workspace"
	applock "iacpack/internal/app/lock"
	"iacpack/internal/diagnostic"
	"iacpack/internal/lockfile"
	"iacpack/internal/oci"
	"iacpack/internal/policy"
)

func TestLockReleaseExampleDeterministic(t *testing.T) {
	root := repoRoot(t)
	src := filepath.Join(root, "examples", "release")
	dir := t.TempDir()
	copyTree(t, src, dir)

	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	manifest := filepath.Join(dir, "iacpack.yaml")
	res := applock.Run(fs, manifest, applock.Options{
		Policy:    policy.MustDefault(),
		Workspace: ws,
		Resolver:  oci.DeniedResolver{},
	})
	if !res.OK {
		t.Fatalf("lock failed: %v", res.Diagnostics)
	}
	first := append([]byte(nil), res.Bytes...)

	// Second lock after recreating workspace with different absolute path.
	dir2 := t.TempDir()
	copyTree(t, src, dir2)
	ws2, err := workspace.New(dir2)
	if err != nil {
		t.Fatal(err)
	}
	res2 := applock.Run(fs, filepath.Join(dir2, "iacpack.yaml"), applock.Options{
		Policy:    policy.MustDefault(),
		Workspace: ws2,
	})
	if !res2.OK {
		t.Fatalf("lock2 failed: %v", res2.Diagnostics)
	}
	if !bytes.Equal(first, res2.Bytes) {
		t.Fatalf("lock bytes differ across absolute paths\n%s\n---\n%s", first, res2.Bytes)
	}

	doc, err := lockfile.Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	if doc.FormatVersion != lockfile.FormatVersion {
		t.Fatal(doc.FormatVersion)
	}
	if len(doc.Inputs) != 1 || doc.Inputs[0].Path != "config/app.yaml" {
		t.Fatalf("inputs=%+v", doc.Inputs)
	}
}

func TestMutableImageRequiresUpdateImages(t *testing.T) {
	dir := t.TempDir()
	manifest := []byte(`apiVersion: iacpack.io/v1alpha1
kind: Project
metadata:
  name: ex
  version: 0.1.0
spec:
  target:
    profile: compose-docker/v1
  mode: development
  images:
    app:
      ref: docker.io/library/nginx:latest
      delivery: pull
  services:
    app:
      image: app
`)
	mustWrite(t, filepath.Join(dir, "iacpack.yaml"), string(manifest))
	ws, err := workspace.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := applock.Run(filesystem.OS{}, filepath.Join(dir, "iacpack.yaml"), applock.Options{
		Policy:    policy.MustDefault(),
		Workspace: ws,
	})
	if !res.OK {
		t.Fatalf("development tag should preview without a digest: %v", res.Diagnostics)
	}
	var warned bool
	for _, d := range res.Diagnostics {
		if d.Code == lockfile.CodeLockImageUnresolved && d.Severity == diagnostic.SeverityWarning {
			warned = true
		}
	}
	if !warned {
		t.Fatal("expected unresolved-image warning")
	}
	fake := oci.MapResolver{
		"docker.io/library/nginx:latest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	res = applock.Run(filesystem.OS{}, filepath.Join(dir, "iacpack.yaml"), applock.Options{
		Policy:       policy.MustDefault(),
		Workspace:    ws,
		UpdateImages: true,
		Resolver:     fake,
	})
	if !res.OK {
		t.Fatalf("fake resolve failed: %v", res.Diagnostics)
	}
	if res.Document.Images["app"].Digest == "" {
		t.Fatal("missing digest")
	}
}

func TestAtomicReplacePreservesOnFailure(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "iacpack.lock.json")
	prior := []byte("{\"formatVersion\":\"iacpack.lock/v1\"}\n")
	if err := os.WriteFile(lockPath, prior, 0o644); err != nil {
		t.Fatal(err)
	}
	fs := failReplaceFS{prior: prior}
	err := fs.WriteFileAtomicReplace(lockPath, []byte("new\n"), 0o644)
	if err == nil {
		t.Fatal("expected failure")
	}
	got, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, prior) {
		t.Fatalf("prior lock mutated: %s", got)
	}
}

type failReplaceFS struct{ prior []byte }

func (failReplaceFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (failReplaceFS) WriteFileAtomicReplace(path string, data []byte, perm os.FileMode) error {
	_ = data
	_ = perm
	return os.ErrPermission
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
