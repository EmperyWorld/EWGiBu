package importpropose

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestProposeSetsNumericUserAndEnvironmentFrom(t *testing.T) {
	prev := imageConfigUser
	imageConfigUser = func(string) (string, bool) { return "1000", true }
	defer func() { imageConfigUser = prev }()

	dir := t.TempDir()
	body := []byte(`services:
  api:
    image: example.invalid/app:1
    environment:
      LLAMA_MODEL: m.gguf
      PORT: "8080"
      ELITEEW_LOG_FILE: /logs/api.log
  beat:
    image: example.invalid/app:1
    environment:
      LLAMA_MODEL: m.gguf
      PORT: "8080"
      ELITEEW_LOG_FILE: /logs/beat.log
`)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	if !res.OK {
		t.Fatalf("propose %v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	svcs := spec["services"].(map[string]any)
	api := svcs["api"].(map[string]any)
	beat := svcs["beat"].(map[string]any)
	if api["user"] != "1000" || beat["user"] != "1000" {
		t.Fatalf("users api=%v beat=%v", api["user"], beat["user"])
	}
	if beat["environmentFrom"] != "api" {
		t.Fatalf("environmentFrom=%v", beat["environmentFrom"])
	}
	env := beat["environment"].(map[string]any)
	if _, ok := env["ELITEEW_LOG_FILE"]; !ok || len(env) != 1 {
		t.Fatalf("beat env=%v", env)
	}

	again := Propose(Request{
		Workspace: dir, Compose: "compose.yaml", Name: "demo",
		ExistingProject: map[string]any{
			"spec": map[string]any{
				"services": map[string]any{
					"api": map[string]any{"user": "1001"},
				},
			},
		},
	})
	api2 := again.Project["spec"].(map[string]any)["services"].(map[string]any)["api"].(map[string]any)
	if api2["user"] != "1001" {
		t.Fatalf("reimport overwrote user: %v", api2["user"])
	}

	imageConfigUser = func(string) (string, bool) { return "0:0", true }
	zero := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	if zero.Project["spec"].(map[string]any)["services"].(map[string]any)["api"].(map[string]any)["user"] != "0:0" {
		t.Fatal("0:0 was not written")
	}

	imageConfigUser = func(string) (string, bool) { return "nginx", true }
	named := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	var sawName bool
	for _, d := range named.Diagnostics {
		if d.Code == diagnostic.CodeImportUserName {
			sawName = true
		}
	}
	if !sawName {
		t.Fatal("expected IMPORT.USER_NAME")
	}
	if _, ok := named.Project["spec"].(map[string]any)["services"].(map[string]any)["beat"].(map[string]any)["user"]; ok {
		t.Fatal("name must not be written")
	}
}
