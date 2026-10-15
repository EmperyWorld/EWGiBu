package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/diagnostic"
)

func TestLockAndValidateLockedReleaseExample(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "release"), dir)
	fs := filesystem.OS{}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"lock", "--file", filepath.Join(dir, "iacpack.yaml"), "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("lock exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "lock.schema.json")

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{
		"validate", "--file", filepath.Join(dir, "iacpack.yaml"),
		"--locked", "--output", "json",
	}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("validate --locked exit=%d stderr=%s", code, stderr.String())
	}

	// Drift: change declared input.
	if err := os.WriteFile(filepath.Join(dir, "config", "app.yaml"), []byte("changed: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{
		"validate", "--file", filepath.Join(dir, "iacpack.yaml"),
		"--locked", "--output", "json",
	}, stdout, stderr, fs)
	if code == cli.ExitOK {
		t.Fatal("expected drift failure")
	}
	if !bytes.Contains(stdout.Bytes(), []byte("LOCK.DRIFT")) {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestValidateSyntaxOnlyWarns(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fs := &testFS{files: map[string][]byte{"m.yaml": data}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"validate", "--file", "m.yaml", "--syntax-only", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(diagnostic.CodeValidateSyntaxOnly)) {
		t.Fatalf("missing syntax-only warning: %s", stdout.String())
	}
}

func TestInspectInputsJSON(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "release"), dir)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"inspect", "inputs", "--file", filepath.Join(dir, "iacpack.yaml"), "--output", "json",
	}, stdout, stderr, filesystem.OS{})
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "inspect_inputs.schema.json")
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["command"] != "inspect inputs" {
		t.Fatalf("command=%v", env["command"])
	}
}

func TestDiscoverExcludesEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"inspect", "--discover", "--workspace", dir, "--output", "json"}, stdout, stderr, filesystem.OS{})
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(".env")) {
		t.Fatalf("discover leaked .env: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Dockerfile")) {
		t.Fatalf("missing Dockerfile suggestion: %s", stdout.String())
	}
}

func copyTreeCLI(t *testing.T, src, dst string) {
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
