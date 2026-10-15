package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
)

func TestImportResourceStubsJSONSchema(t *testing.T) {
	root := repoRoot(t)
	ws := filepath.Join(root, "testdata", "import", "happy")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	prop := &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"import", "propose",
		"--workspace", ws,
		"--compose", "compose.yaml",
		"--output", "json",
	}, prop, stderr, nil)
	if code != cli.ExitOK {
		t.Fatalf("propose exit=%d", code)
	}
	var env map[string]any
	if err := json.Unmarshal(prop.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	result := env["result"].(map[string]any)
	project := result["project"]
	projPath := filepath.Join(t.TempDir(), "project.json")
	raw, _ := json.Marshal(project)
	if err := os.WriteFile(projPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	code = cli.ExecuteWithFS([]string{
		"import", "resource-stubs",
		"--workspace", ws,
		"--project-file", projPath,
		"--output", "json",
	}, stdout, stderr, nil)
	if code != cli.ExitOK {
		t.Fatalf("stubs exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "import_resource_stubs.schema.json")
}
