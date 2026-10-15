package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
)

func TestBuildDirectoryReleaseDeterministic(t *testing.T) {
	root := repoRoot(t)
	src := filepath.Join(root, "examples", "release")
	dir := t.TempDir()
	copyTreeCLI(t, src, dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"lock", "--file", file, "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("lock: %d %s", code, stderr.String())
	}

	out1 := filepath.Join(t.TempDir(), "dist1")
	out2 := filepath.Join(t.TempDir(), "dist2")
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "directory", "--output", out1, "--output", "json",
	}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("build1: %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "build.schema.json")

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "directory", "--output", out2,
	}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("build2: %d %s", code, stderr.String())
	}

	assertTreesEqual(t, out1, out2)

	// Required files
	for _, p := range []string{
		"manifest.canonical.json",
		"deployment.json",
		"iacpack.lock.json",
		"policy.compile.json",
		"rendered/compose.preview.yaml",
		"payload/app-config/app.yaml",
		"docs/README.md",
		"docs/architecture.mmd",
		"config/.env.example",
	} {
		if _, err := os.Stat(filepath.Join(out1, filepath.FromSlash(p))); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out1, "config", ".env")); err == nil {
		t.Fatal(".env must not exist")
	}

	// Secret canary
	canary := []byte("SUPER_SECRET_CANARY_VALUE_9f3a")
	if err := filepath.Walk(out1, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, canary) {
			t.Errorf("canary in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Refuse overwrite
	code = cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "directory", "--output", out1,
	}, &bytes.Buffer{}, &bytes.Buffer{}, fs)
	if code == cli.ExitOK {
		t.Fatal("expected refuse overwrite")
	}

	maybeComposeConfig(t, filepath.Join(out1, "rendered", "compose.preview.yaml"))
}

func TestBuildMinimalAndUndeclaredExcluded(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	// undeclared file must not appear in output
	if err := os.WriteFile(filepath.Join(dir, "undeclared.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatalf("lock %d", code)
	}
	out := filepath.Join(t.TempDir(), "out")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "directory", "--output", out, "--output", "json",
	}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("build %d %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "undeclared.txt")); err == nil {
		t.Fatal("undeclared file leaked into artifact")
	}
	if _, err := os.Stat(filepath.Join(out, "payload")); err == nil {
		t.Fatal("unexpected payload dir for minimal")
	}
}

func TestBuildLockDriftFails(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "release"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "app.yaml"), []byte("changed: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "directory", "--output", out, "--output", "json",
	}, stdout, stderr, fs)
	if code == cli.ExitOK {
		t.Fatal("expected drift failure")
	}
	if !strings.Contains(stdout.String()+stderr.String(), "LOCK.DRIFT") &&
		!strings.Contains(stdout.String()+stderr.String(), "INPUT.CHANGED") {
		t.Fatalf("want drift diagnostic, got stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func assertTreesEqual(t *testing.T, a, b string) {
	t.Helper()
	var pathsA, pathsB []string
	_ = filepath.Walk(a, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(a, path)
		pathsA = append(pathsA, filepath.ToSlash(rel))
		return nil
	})
	_ = filepath.Walk(b, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(b, path)
		pathsB = append(pathsB, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(pathsA)
	sort.Strings(pathsB)
	if strings.Join(pathsA, "\n") != strings.Join(pathsB, "\n") {
		t.Fatalf("path set mismatch\nA=%v\nB=%v", pathsA, pathsB)
	}
	for _, rel := range pathsA {
		ba, err := os.ReadFile(filepath.Join(a, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		bb, err := os.ReadFile(filepath.Join(b, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ba, bb) {
			t.Fatalf("content mismatch %s", rel)
		}
	}
}

func maybeComposeConfig(t *testing.T, composeFile string) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Log("docker not available; skipping compose config gate")
		return
	}
	cmd := exec.Command("docker", "compose", "-f", composeFile, "config", "--quiet")
	cmd.Env = append(os.Environ(), "DOCKER_CLI_HINTS=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Compose may reject absolute-less bind paths depending on version; log and soft-fail only if binary missing plugin.
		if bytes.Contains(out, []byte("is not a Compose")) || bytes.Contains(out, []byte("unknown command")) {
			t.Logf("docker compose plugin unavailable: %s", out)
			return
		}
		t.Fatalf("docker compose config: %v\n%s", err, out)
	}
}
