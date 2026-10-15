package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
	"iacpack/internal/adapters/filesystem"
)

func TestInspectGraphReleaseJSONAndMermaid(t *testing.T) {
	root := repoRoot(t)
	dir := t.TempDir()
	copyTreeCLI(t, filepath.Join(root, "examples", "release"), dir)
	fs := filesystem.OS{}
	file := filepath.Join(dir, "iacpack.yaml")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{"inspect", "graph", "--file", file, "--output", "json"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("json exit=%d stderr=%s", code, stderr.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "inspect_graph.schema.json")
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["command"] != "inspect graph" {
		t.Fatalf("command=%v", env["command"])
	}
	result := env["result"].(map[string]any)
	if result["mermaid"] == nil || result["mermaid"] == "" {
		t.Fatal("missing mermaid in json result")
	}

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{"inspect", "graph", "--file", file, "--output", "mermaid"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("mermaid exit=%d stderr=%s", code, stderr.String())
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("flowchart TD\n")) {
		t.Fatalf("mermaid=%s", stdout.String())
	}

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{"inspect", "graph", "--file", file, "--output", "text"}, stdout, stderr, fs)
	if code != cli.ExitOK {
		t.Fatalf("text exit=%d stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("deploymentHash:")) {
		t.Fatalf("text=%s", stdout.String())
	}

	// Golden-ish stability: two runs identical.
	first := append([]byte(nil), stdout.Bytes()...)
	stdout2, stderr2 := &bytes.Buffer{}, &bytes.Buffer{}
	code = cli.ExecuteWithFS([]string{"inspect", "graph", "--file", file, "--output", "text"}, stdout2, stderr2, fs)
	if code != cli.ExitOK {
		t.Fatal(code)
	}
	if !bytes.Equal(first, stdout2.Bytes()) {
		t.Fatal("text not stable across runs")
	}
	_ = os.Stdout
}
