package importpropose_test

import (
	"os"
	"path/filepath"
	"testing"

	"iacpack/internal/app/importpropose"
)

func TestProposeKeepsRequirepassSecretAndUserAndURL(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  redis:
    image: redis:7.2
    command:
      - redis-server
      - --requirepass
      - "${REDIS_PASSWORD}"
  paperless:
    image: paperless:1
    environment:
      PAPERLESS_REDIS: "redis://:${PAPERLESS_REDIS_PASSWORD}@redis:6379/1"
  frontend:
    image: eliteew-frontend:1
    user: "${STACK0_FRONTEND_USER:-0:0}"
`)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	spec := res.Project["spec"].(map[string]any)
	svcs := spec["services"].(map[string]any)
	redis := svcs["redis"].(map[string]any)
	cmd := redis["command"].([]any)
	if len(cmd) != 3 {
		t.Fatalf("redis command=%#v", cmd)
	}
	if cmd[1] != "--requirepass" {
		t.Fatalf("flag dropped: %#v", cmd)
	}
	sec, ok := cmd[2].(map[string]any)
	if !ok || sec["secret"] != "redis-password" {
		t.Fatalf("secret argv=%#v", cmd[2])
	}
	paper := svcs["paperless"].(map[string]any)
	env := paper["environment"].(map[string]any)
	url := env["PAPERLESS_REDIS"].(map[string]any)
	parts, _ := url["parts"].([]any)
	if len(parts) != 3 {
		t.Fatalf("url=%#v", url)
	}
	mid := parts[1].(map[string]any)
	if mid["secret"] != "paperless-redis-password" {
		t.Fatalf("mid=%#v", parts)
	}
	front := svcs["frontend"].(map[string]any)
	if front["user"] != "0:0" {
		t.Fatalf("user=%#v", front["user"])
	}
	secrets := spec["secrets"].(map[string]any)
	if _, ok := secrets["redis-password"]; !ok {
		t.Fatalf("secrets=%v", secrets)
	}
}
