package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
)

func TestReleaseInitTemplatesJSONSchemaAndGolden(t *testing.T) {
	dir := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"release", "init-templates",
		"--workspace", dir,
		"--dry-run",
		"--output", "json",
	}, stdout, stderr, nil)
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	raw := stdout.Bytes()
	validateAgainstCheckedInSchema(t, raw, "release_init_templates.schema.json")

	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env["command"] != "release init-templates" || env["ok"] != true {
		t.Fatalf("envelope=%v", env)
	}
	result := env["result"].(map[string]any)
	if result["wrote"] != false {
		t.Fatal("dry-run must not write")
	}
	files := result["files"].([]any)
	if len(files) != 4 {
		t.Fatalf("files=%d", len(files))
	}

	root := repoRoot(t)
	goldenPath := filepath.Join(root, "testdata", "golden", "release_init_templates", "dry_run.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		// Normalize: keep force/wrote/file path+action+bytes; sha256 is stable for empty ws
		norm, _ := json.MarshalIndent(env, "", "  ")
		if err := os.WriteFile(goldenPath, append(norm, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (set UPDATE_GOLDEN=1): %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	gotFiles, _ := json.Marshal(result["files"])
	wantFiles, _ := json.Marshal(want["result"].(map[string]any)["files"])
	if string(gotFiles) != string(wantFiles) {
		t.Fatalf("files mismatch\ngot=%s\nwant=%s", gotFiles, wantFiles)
	}
}

func TestReleaseInitTemplatesWriteAndRefuse(t *testing.T) {
	dir := t.TempDir()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"release", "init-templates",
		"--workspace", dir,
		"--output", "json",
	}, stdout, stderr, nil)
	if code != cli.ExitOK {
		t.Fatalf("write exit=%d stderr=%s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "release_init_templates.schema.json")
	if _, err := os.Stat(filepath.Join(dir, ".github/workflows/iacpack-ci.yml")); err != nil {
		t.Fatal(err)
	}

	stdout2 := &bytes.Buffer{}
	code2 := cli.ExecuteWithFS([]string{
		"release", "init-templates",
		"--workspace", dir,
		"--output", "json",
	}, stdout2, &bytes.Buffer{}, nil)
	// Second run should succeed with skips (identical content)
	if code2 != cli.ExitOK {
		t.Fatalf("skip exit=%d out=%s", code2, stdout2.String())
	}
}
