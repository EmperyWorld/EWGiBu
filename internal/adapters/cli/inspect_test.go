package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
)

func TestInspectManifestMinimalJSON(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testdata/manifests/positive/minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fs := &testFS{files: map[string][]byte{"minimal.yaml": data}}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"inspect", "manifest", "--file", "minimal.yaml", "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "inspect_manifest.schema.json")
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["ok"] != true || env["command"] != "inspect manifest" {
		t.Fatalf("env=%v", env)
	}
	result := env["result"].(map[string]any)
	if result["semanticHash"] == "" {
		t.Fatal("missing semanticHash")
	}
}

func TestInspectManifestMissingFileUsage(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWith([]string{"inspect", "manifest"}, stdout, stderr)
	if code != cli.ExitUsage {
		t.Fatalf("exit=%d", code)
	}
}
