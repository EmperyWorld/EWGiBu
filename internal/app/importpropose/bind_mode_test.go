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

func TestProposeSelinuxBindStaysWritableAndStringCommand(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{
		"docker/stack",
		"data/stack1/postgresql",
		"data/stack1/redis",
		"logs/stack1/chroma",
	} {
		if err := os.MkdirAll(filepath.Join(dir, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "data/stack1/postgresql.conf"), []byte("# conf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte(`
services:
  postgres:
    image: docker.io/library/nginx:1.27
    volumes:
      - ../data/stack1/postgresql:/var/lib/postgresql/data:z
      - ../logs/stack1/chroma:/logs:Z
      - ../data/stack1/redis:/data
      - ../data/stack1/postgresql.conf:/etc/postgresql.conf:ro,z
  keycloak:
    image: docker.io/library/nginx:1.27
    command: start
`)
	if err := os.WriteFile(filepath.Join(dir, "docker/stack/stack1.yml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: dir, Compose: "docker/stack/stack1.yml", Name: "binds",
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	for _, d := range res.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("error %s: %s", d.Code, d.Message)
		}
		if d.Code == diagnostic.CodeImportUnsupportedField && (strings.Contains(d.Message, "string command") || strings.Contains(d.Message, "read")) {
			t.Fatalf("unexpected %s", d.Message)
		}
	}
	spec := res.Project["spec"].(map[string]any)
	pg := spec["services"].(map[string]any)["postgres"].(map[string]any)
	mounts := pg["mounts"].([]any)
	if len(mounts) != 4 {
		t.Fatalf("mounts=%v", mounts)
	}
	byTarget := map[string]map[string]any{}
	for _, raw := range mounts {
		m := raw.(map[string]any)
		byTarget[m["target"].(string)] = m
	}
	for _, tgt := range []string{"/var/lib/postgresql/data", "/logs", "/data"} {
		m := byTarget[tgt]
		if m == nil || m["readOnly"] != false {
			t.Fatalf("%s mount=%v", tgt, m)
		}
	}
	conf := byTarget["/etc/postgresql.conf"]
	if conf == nil || conf["readOnly"] != true {
		t.Fatalf("ro mount=%v", conf)
	}
	kc := spec["services"].(map[string]any)["keycloak"].(map[string]any)
	cmd, _ := kc["command"].([]any)
	if len(cmd) != 1 || cmd[0] != "start" {
		t.Fatalf("command=%#v", kc["command"])
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
	dep, err := compile.ToIR(proj)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := compose.Render(dep, compose.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, vol := range rendered.Services["postgres"].Volumes {
		switch vol.Target {
		case "/var/lib/postgresql/data", "/logs", "/data":
			if vol.ReadOnly {
				t.Fatalf("%s rendered read-only: %+v", vol.Target, vol)
			}
		case "/etc/postgresql.conf":
			if !vol.ReadOnly {
				t.Fatalf("conf must stay read-only: %+v", vol)
			}
		default:
			t.Fatalf("unexpected volume %+v", vol)
		}
	}
	if got := rendered.Services["keycloak"].Command; len(got) != 1 || got[0] != "start" {
		t.Fatalf("rendered command=%v", got)
	}
	yamlBytes, err := compose.MarshalYAML(rendered)
	if err != nil {
		t.Fatal(err)
	}
	text := string(yamlBytes)
	if !strings.Contains(text, "command:\n") || !strings.Contains(text, "- start\n") {
		t.Fatalf("rendered yaml missing command:\n%s", text)
	}
}
