package importpropose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/manifest/loader"
	v1alpha1 "iacpack/internal/manifest/v1alpha1"
)

func TestSplitComposeCommandString(t *testing.T) {
	words, ok := splitComposeCommandString("start")
	if !ok || len(words) != 1 || words[0] != "start" {
		t.Fatalf("start: %#v ok=%v", words, ok)
	}
	words, ok = splitComposeCommandString("start --optimized")
	if !ok || len(words) != 2 || words[0] != "start" || words[1] != "--optimized" {
		t.Fatalf("flags: %#v", words)
	}
	words, ok = splitComposeCommandString("echo a | tee b")
	if !ok || len(words) != 3 || words[0] != "/bin/sh" || words[1] != "-c" || words[2] != "echo a | tee b" {
		t.Fatalf("shell: %#v", words)
	}
	if _, ok = splitComposeCommandString(`"unterminated`); ok {
		t.Fatal("unbalanced quotes")
	}
}

func TestNormalizeResourceCPUsComposeDecimals(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2.0": "2",
		"4.0": "4",
		"1.0": "1",
		"0.5": "0.5",
		"2":   "2",
	}
	for in, want := range cases {
		got, ok := v1alpha1.NormalizeResourceCPUs(in)
		if !ok || got != want {
			t.Fatalf("NormalizeResourceCPUs(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
}

func TestManifestSafeArgvListOmitsRequirepassAndEmptyLogfile(t *testing.T) {
	t.Parallel()
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  redis:
    command:
      - redis-server
      - --requirepass
      - "${REDIS_PASSWORD}"
      - --logfile
      - ""
`))
	n := doc.Root
	svc := n.GetMap("services").GetMap("redis")
	cmd := svc.GetMap("command")
	arr, diags := manifestSafeArgvList(cmd, "f.yml", "/services/redis/command", "service redis command")
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1 warning", len(diags))
	}
	if len(arr) != 1 || arr[0] != "redis-server" {
		t.Fatalf("argv = %#v, want [redis-server]", arr)
	}
}

func TestImportHealthcheckCMDShell(t *testing.T) {
	t.Parallel()
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  api:
    healthcheck:
      test: ["CMD-SHELL", "curl -sf http://127.0.0.1:8080/health >/dev/null"]
`))
	hc := doc.Root.GetMap("services").GetMap("api").GetMap("healthcheck").GetMap("test")
	arr, _ := importHealthcheckCommand(hc, "f.yml", "/services/api", "api")
	if len(arr) != 3 || arr[0] != "/bin/sh" || arr[1] != "-c" {
		t.Fatalf("command = %#v", arr)
	}
	if arr[2] != "curl -sf http://127.0.0.1:8080/health >/dev/null" {
		t.Fatalf("script = %v", arr[2])
	}
}

func TestImportCommandMaxTokensKeepsDefault(t *testing.T) {
	t.Parallel()
	if isSecretLikeKey("LLAMA_CHAT_IMAGE_MAX_TOKENS") || isSecretLikeKey("llama_chat_image_max_tokens") {
		t.Fatal("MAX_TOKENS is a count, not a secret token")
	}
	if !isSecretLikeKey("API_TOKEN") || !isSecretLikeKey("POSTGRES_PASSWORD") {
		t.Fatal("credential names must stay secret-like")
	}
	doc := loader.ParseComposeInput("stack2.yml", []byte(`
services:
  llama-chat:
    command:
      - --image-max-tokens
      - "${LLAMA_CHAT_IMAGE_MAX_TOKENS:-512}"
      - --host
`))
	cmd := doc.Root.GetMap("services").GetMap("llama-chat").GetMap("command")
	params := map[string]any{}
	arr, diags := importCommandList(cmd, "stack2.yml", "/services/llama-chat/command", "service llama-chat command", params, nil)
	for _, d := range diags {
		if strings.Contains(d.Message, "omitted") {
			t.Fatalf("omitted: %s", d.Message)
		}
	}
	if len(arr) != 3 || arr[0] != "--image-max-tokens" || arr[2] != "--host" {
		t.Fatalf("argv=%#v", arr)
	}
	parts, _ := arr[1].(map[string]any)["parts"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["parameter"] != "llama-chat-image-max-tokens" {
		t.Fatalf("parts=%#v", arr[1])
	}
	p, _ := params["llama-chat-image-max-tokens"].(map[string]any)
	if p["default"] != "512" {
		t.Fatalf("param=%v", p)
	}
}

func TestImportHealthcheckEscapedDollarIsShellVar(t *testing.T) {
	t.Parallel()
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  db:
    healthcheck:
      test: ['CMD-SHELL', 'pg_isready -U "$${POSTGRES_USER}" -d "$${POSTGRES_DB}"']
`))
	hc := doc.Root.GetMap("services").GetMap("db").GetMap("healthcheck").GetMap("test")
	arr, diags := importHealthcheckCommand(hc, "f.yml", "/services/db", "db")
	if len(diags) != 0 {
		t.Fatalf("diags=%v", diags)
	}
	if len(arr) != 3 || arr[0] != "/bin/sh" || arr[1] != "-c" {
		t.Fatalf("command=%#v", arr)
	}
	const want = `pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"`
	if arr[2] != want {
		t.Fatalf("script=%q", arr[2])
	}
}

func TestImportHealthcheckRedisWithoutAuth(t *testing.T) {
	t.Parallel()
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  redis:
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"${REDIS_PASSWORD}\" ping | grep -q PONG"]
`))
	n := doc.Root
	svc := n.GetMap("services").GetMap("redis")
	hc := svc.GetMap("healthcheck").GetMap("test")
	arr, diags := importHealthcheckCommand(hc, "f.yml", "/services/redis", "redis")
	if arr != nil {
		t.Fatalf("probe without password must not render: %#v", arr)
	}
	if len(diags) == 0 || !strings.Contains(diags[0].Message, "REDIS_PASSWORD") {
		t.Fatalf("diags=%v", diags)
	}
}

func TestImportRewritesContainerNameDNS(t *testing.T) {
	dir := t.TempDir()
	body := []byte(`
services:
  seaweedfs:
    image: seaweedfs:1
    container_name: stack1-seaweedfs
  api:
    image: api:1
    environment:
      AWS_S3_ENDPOINT: http://stack1-seaweedfs:8333
      CORS_ORIGINS: "${CORS_ORIGINS}"
`)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	res := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "demo"})
	spec := res.Project["spec"].(map[string]any)
	api := spec["services"].(map[string]any)["api"].(map[string]any)
	env := api["environment"].(map[string]any)
	if env["AWS_S3_ENDPOINT"].(map[string]any)["literal"] != "http://seaweedfs:8333" {
		t.Fatalf("endpoint=%#v", env["AWS_S3_ENDPOINT"])
	}
	if _, ok := env["CORS_ORIGINS"]; !ok {
		t.Fatalf("env keys=%v", env)
	}
	if _, renamed := env["CORS_ALLOWED_ORIGINS"]; renamed {
		t.Fatal("env key was renamed")
	}
}

func TestHealthcheckUnresolvedParameterIsError(t *testing.T) {
	doc := loader.ParseComposeInput("f.yml", []byte(`
services:
  api:
    healthcheck:
      test: ["CMD", "curl", "-f", "${API_HEALTH_URL}"]
`))
	hc := doc.Root.GetMap("services").GetMap("api").GetMap("healthcheck").GetMap("test")
	arr, diags := importHealthcheckCommand(hc, "f.yml", "/services/api", "api")
	if arr != nil {
		t.Fatalf("partial command rendered: %#v", arr)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "API_HEALTH_URL") {
		t.Fatalf("diags=%v", diags)
	}
}

func TestImportCommandKeepsEmptyLogfileArg(t *testing.T) {
	doc := loader.ParseComposeInput("stack1.yml", []byte(`
services:
  redis:
    command:
      - redis-server
      - --logfile
      - ""
`))
	cmd := doc.Root.GetMap("services").GetMap("redis").GetMap("command")
	arr, diags := importCommandList(cmd, "stack1.yml", "/services/redis/command", "service redis command", nil, nil)
	for _, d := range diags {
		if strings.Contains(d.Message, "omitted") {
			t.Fatalf("dropped: %s", d.Message)
		}
	}
	if len(arr) != 3 || arr[1] != "--logfile" {
		t.Fatalf("argv=%#v", arr)
	}
	lit, _ := arr[2].(map[string]any)
	if lit["literal"] != "" {
		t.Fatalf("empty arg=%#v", arr[2])
	}
}
