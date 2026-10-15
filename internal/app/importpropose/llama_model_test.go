package importpropose_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/compile"
	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
	"iacpack/internal/render/compose"
)

func TestProposeLlamaModelBindAndCommand(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  llama-chat:
    image: docker.io/library/nginx:1.27
    volumes:
      - ${STACK2_LLAMA_MODELS_PATH:?STACK2_LLAMA_MODELS_PATH not set}:/models:ro,z
    command:
      - -m
      - /models/${LLAMA_CHAT_FILE:?LLAMA_CHAT_FILE not set}
      - --no-mmproj
`)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{Workspace: dir, Compose: "compose.yaml", Name: "llama"})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportPolicyDenied {
			t.Fatalf("host parameter bind refused: %s", d.Message)
		}
		if strings.Contains(d.Message, "argv element with Compose interpolation omitted") {
			t.Fatalf("unexpected %s", d.Message)
		}
	}
	spec := res.Project["spec"].(map[string]any)
	if _, ok := spec["resources"]; ok {
		t.Fatalf("model dir must not be a bundled resource: %v", spec["resources"])
	}
	svc := spec["services"].(map[string]any)["llama-chat"].(map[string]any)
	mounts := svc["mounts"].([]any)
	if len(mounts) != 1 {
		t.Fatalf("mounts=%v", mounts)
	}
	m := mounts[0].(map[string]any)
	if m["target"] != "/models" || m["readOnly"] != true {
		t.Fatalf("mount=%v", m)
	}
	host := m["host"].(map[string]any)
	if host["parameter"] != "stack2-llama-models-path" {
		t.Fatalf("host=%v", host)
	}
	cmd := svc["command"].([]any)
	if cmd[0] != "-m" || cmd[2] != "--no-mmproj" {
		t.Fatalf("command=%v", cmd)
	}
	parts := cmd[1].(map[string]any)["parts"].([]any)
	if parts[0] != "/models/" || parts[1].(map[string]any)["parameter"] != "llama-chat-file" {
		t.Fatalf("parts=%v", parts)
	}
	raw, err := json.Marshal(res.Project)
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.Parse("draft.json", raw)
	proj, diags := v1alpha1.Decode(doc)
	if diagnostic.HasError(diags) {
		t.Fatalf("decode: %v", diags)
	}
	ir, err := compile.ToIR(proj)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := compose.Render(ir, compose.RenderOptions{DeployValues: map[string]string{
		"stack2-llama-models-path": "/home/owned/llama-test-models",
		"llama-chat-file":          "gemma-4-26B.gguf",
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := rendered.Services["llama-chat"]
	if len(got.Command) != 3 || got.Command[1] != "/models/gemma-4-26B.gguf" {
		t.Fatalf("rendered command=%v", got.Command)
	}
	if len(got.Volumes) != 1 || got.Volumes[0].Source != "/home/owned/llama-test-models" || got.Volumes[0].Target != "/models" {
		t.Fatalf("rendered volumes=%v", got.Volumes)
	}
}
