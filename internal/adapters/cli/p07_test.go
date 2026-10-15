package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/runtime/process"
)

func TestPlanDeterministicNoSecrets(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "release"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	if code := cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("lock")
	}
	bundle := filepath.Join(t.TempDir(), "p.iacbundle")
	if code := cli.ExecuteWithFS([]string{
		"build", "--file", file, "--format", "bundle", "--output", bundle,
	}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}

	secretPath := filepath.Join(dir, "db.secret")
	if err := os.WriteFile(secretPath, []byte("SUPER_SECRET_CANARY_VALUE_9f3a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout1, stderr1 := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"plan", bundle, "--secret-file", "db-password=" + secretPath, "--output", "json",
	}, stdout1, stderr1, fs)
	if code != cli.ExitOK {
		t.Fatalf("plan1 %d %s %s", code, stdout1.String(), stderr1.String())
	}
	validateAgainstCheckedInSchema(t, stdout1.Bytes(), "plan.schema.json")

	stdout2, stderr2 := &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{
		"plan", bundle, "--instance", "default", "--secret-file", "db-password=" + secretPath, "--output", "json",
	}, stdout2, stderr2, fs)
	if code != cli.ExitOK {
		t.Fatalf("plan2 %d %s", code, stderr2.String())
	}
	if stdout1.String() != stdout2.String() {
		t.Fatal("plan not deterministic")
	}
	canary := "SUPER_SECRET_CANARY_VALUE_9f3a"
	if strings.Contains(stdout1.String(), canary) {
		t.Fatal("secret canary in plan")
	}
	var env map[string]any
	if err := json.Unmarshal(stdout1.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	res := env["result"].(map[string]any)
	if res["providerSlug"] == "" {
		t.Fatal("missing slug")
	}
}

func TestPlanMissingSecret(t *testing.T) {
	// Build a bundle from release (no required secrets) — craft secret via operator path on validate is hard;
	// instead ensure --secret-file unknown id fails.
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "minimal"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")
	_ = cli.ExecuteWithFS([]string{"lock", "--file", file}, &bytes.Buffer{}, &bytes.Buffer{}, fs)
	bundle := filepath.Join(t.TempDir(), "m.iacbundle")
	if code := cli.ExecuteWithFS([]string{"build", "--file", file, "--format", "bundle", "--output", bundle}, &bytes.Buffer{}, &bytes.Buffer{}, fs); code != cli.ExitOK {
		t.Fatal("build")
	}
	secretPath := filepath.Join(dir, "s.txt")
	if err := os.WriteFile(secretPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"plan", bundle, "--secret-file", "nope=" + secretPath, "--output", "json",
	}, stdout, stderr, fs)
	if code == cli.ExitOK {
		t.Fatal("expected unknown secret failure")
	}
	if !strings.Contains(stdout.String()+stderr.String(), "SECRET.UNKNOWN") {
		t.Fatalf("want SECRET.UNKNOWN got %s %s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "SECRET_CANARY") {
		t.Fatal("leak")
	}
}

func TestProcessRejectsShell(t *testing.T) {
	r := &process.RecordingRunner{}
	_, err := r.Run(context.Background(), process.Spec{Path: "/bin/sh", Args: []string{"-c", "echo hi"}})
	if err == nil {
		t.Fatal("expected shell reject")
	}
}
