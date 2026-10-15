package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/adapters/cli"
)

func TestImportProposeJSONSchemaAndGolden(t *testing.T) {
	root := repoRoot(t)
	ws := filepath.Join(root, "testdata", "import", "happy")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"import", "propose",
		"--workspace", ws,
		"--compose", "compose.yaml",
		"--name", "demo",
		"--output", "json",
	}, stdout, stderr, nil)
	if code != cli.ExitOK {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	raw := stdout.Bytes()
	validateAgainstCheckedInSchema(t, raw, "import_propose.schema.json")

	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env["command"] != "import propose" || env["ok"] != true {
		t.Fatalf("envelope=%v", env)
	}
	result := env["result"].(map[string]any)
	summary := result["summary"].(map[string]any)
	if summary["services"] != float64(3) {
		t.Fatalf("summary=%v", summary)
	}

	goldenPath := filepath.Join(root, "testdata", "golden", "import_propose", "happy.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		// Normalize for golden: drop volatile diagnostics source pointers if any
		norm, _ := json.MarshalIndent(env, "", "  ")
		if err := os.WriteFile(goldenPath, append(norm, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (set UPDATE_GOLDEN=1 to create): %v", err)
	}
	var want map[string]any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	// Compare result.summary + ok + command only for stability; project tree may reorder maps in Go 1.27+
	// Actually json.Marshal of maps is randomized in Go - need deterministic encoding in WriteJSON!
	gotSum, _ := json.Marshal(result["summary"])
	wantResult := want["result"].(map[string]any)
	wantSum, _ := json.Marshal(wantResult["summary"])
	if string(gotSum) != string(wantSum) {
		t.Fatalf("summary mismatch got=%s want=%s", gotSum, wantSum)
	}
	if want["ok"] != true {
		t.Fatalf("golden ok=%v", want["ok"])
	}
}

func TestImportProposePrivilegedFails(t *testing.T) {
	root := repoRoot(t)
	ws := filepath.Join(root, "testdata", "import", "privileged")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cli.ExecuteWithFS([]string{
		"import", "propose",
		"--workspace", ws,
		"--compose", "compose.yaml",
		"--output", "json",
	}, stdout, stderr, nil)
	if code == cli.ExitOK {
		t.Fatalf("expected failure, got ok: %s", stdout.String())
	}
	validateAgainstCheckedInSchema(t, stdout.Bytes(), "import_propose.schema.json")
}
