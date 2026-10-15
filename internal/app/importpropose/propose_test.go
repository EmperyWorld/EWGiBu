package importpropose_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
)

func repoFixture(t *testing.T, rel string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "import", rel)
}

func TestProposeHappy(t *testing.T) {
	ws := repoFixture(t, "happy")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "demo",
	})
	if !res.OK {
		t.Fatalf("expected ok, diags=%v", res.Diagnostics)
	}
	if res.Summary.Services != 3 {
		t.Fatalf("services=%d", res.Summary.Services)
	}
	if res.Summary.Images != 3 {
		t.Fatalf("images=%d", res.Summary.Images)
	}
	if res.Summary.Networks != 2 {
		t.Fatalf("networks=%d", res.Summary.Networks)
	}
	spec := res.Project["spec"].(map[string]any)
	if spec["mode"] != "development" {
		t.Fatalf("mode=%v want development (build present)", spec["mode"])
	}
	nets := spec["networks"].(map[string]any)
	backend := nets["backend"].(map[string]any)
	ext := backend["external"].(map[string]any)
	if ext["name"] != "edge-net" {
		t.Fatalf("external name=%v", ext["name"])
	}
}

func TestProposeDockerfileOnly(t *testing.T) {
	ws := repoFixture(t, "happy")
	res := importpropose.Propose(importpropose.Request{
		Workspace:   ws,
		Dockerfiles: []string{"app/Dockerfile"},
	})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	if res.Summary.Images != 1 || res.Summary.Services != 0 {
		t.Fatalf("summary=%+v", res.Summary)
	}
	spec := res.Project["spec"].(map[string]any)
	if spec["mode"] != "development" {
		t.Fatalf("mode=%v", spec["mode"])
	}
}

func TestProposePrivilegedDenied(t *testing.T) {
	ws := repoFixture(t, "privileged")
	res := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "compose.yaml"})
	if res.OK {
		t.Fatal("expected not ok")
	}
	assertCode(t, res.Diagnostics, diagnostic.CodeImportPolicyDenied)
}

func TestProposeEnvFileSuggestsSecretStub(t *testing.T) {
	ws := repoFixture(t, "envfile")
	res := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "compose.yaml"})
	if !res.OK {
		t.Fatalf("expected ok with env_file suggestion, diags=%v", res.Diagnostics)
	}
	assertCode(t, res.Diagnostics, diagnostic.CodeImportSecretLike)
	found := false
	for _, s := range res.SecretSuggestions {
		if s.Source == "env_file" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected env_file suggestion, got %+v", res.SecretSuggestions)
	}
	assertNoCanary(t, res)
}

func TestProposeDockerSockDenied(t *testing.T) {
	ws := repoFixture(t, "bind-host")
	res := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "compose.yaml"})
	if !res.OK {
		t.Fatalf("absolute docker.sock bind is a warning; diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	if _, ok := spec["services"].(map[string]any)["bad"]; !ok {
		t.Fatal("service dropped with absolute bind")
	}
	var warn bool
	for _, d := range res.Diagnostics {
		if d.Code == diagnostic.CodeImportPolicyDenied && d.Severity == diagnostic.SeverityWarning {
			warn = true
		}
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected error %s %s", d.Code, d.Message)
		}
	}
	if !warn {
		t.Fatal("expected policy warning")
	}
	strict := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "compose.yaml", Strict: true})
	if strict.OK {
		t.Fatal("expected ok=false with --import-strict")
	}
	if _, ok := strict.Project["spec"].(map[string]any)["services"].(map[string]any)["bad"]; !ok {
		t.Fatal("strict absolute bind must keep the service")
	}
}

func TestProposeInterpolation(t *testing.T) {
	ws := repoFixture(t, "interp")
	res := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "compose.yaml"})
	if !res.OK {
		t.Fatalf("expected ok with parameter stubs, diags=%v", res.Diagnostics)
	}
	assertCode(t, res.Diagnostics, diagnostic.CodeImportInterpolation)
	assertCode(t, res.Diagnostics, diagnostic.CodeImportSecretLike)
	spec := res.Project["spec"].(map[string]any)
	params, _ := spec["parameters"].(map[string]any)
	if _, ok := params["public-host"]; !ok {
		t.Fatalf("expected public-host parameter, params=%v", params)
	}
	svcs := spec["services"].(map[string]any)
	api := svcs["api"].(map[string]any)
	env := api["environment"].(map[string]any)
	pw, _ := env["DB_PASSWORD"].(map[string]any)
	if pw["secret"] != "db-password" {
		t.Fatalf("DB_PASSWORD=%v", env["DB_PASSWORD"])
	}
	secrets, _ := spec["secrets"].(map[string]any)
	if _, ok := secrets["db-password"]; !ok {
		t.Fatalf("secrets=%v", secrets)
	}
	if _, ok := env["HOST"]; !ok {
		t.Fatal("HOST should map to parameter")
	}
	if len(res.ParameterGaps) == 0 {
		t.Fatal("expected parameter gaps")
	}
	foundSecret := false
	for _, s := range res.SecretSuggestions {
		if s.Source == "env_key" && s.EnvKey == "DB_PASSWORD" {
			foundSecret = true
			break
		}
	}
	if !foundSecret {
		t.Fatalf("expected DB_PASSWORD secret suggestion, got %+v", res.SecretSuggestions)
	}
}

func TestProposeSecretsOnboardingCanary(t *testing.T) {
	ws := repoFixture(t, "secrets-onboard")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "compose.yaml",
		Name:      "onboard",
	})
	if !res.OK {
		t.Fatalf("expected ok, diags=%v", res.Diagnostics)
	}
	assertNoCanary(t, res)
	if len(res.SecretSuggestions) < 3 {
		t.Fatalf("expected env_file + env_key + compose_secret suggestions, got %+v", res.SecretSuggestions)
	}
	applied, err := importpropose.ApplySuggestions(res.Project, importpropose.ApplyOptions{
		SecretIDs:     suggestionIDsForTest(res.SecretSuggestions),
		Suggestions:   res.SecretSuggestions,
		AttachMounts:  true,
		ParamDefaults: map[string]any{"public-host": "example.local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := applied["spec"].(map[string]any)
	secrets, _ := spec["secrets"].(map[string]any)
	if len(secrets) == 0 {
		t.Fatal("expected secrets after apply")
	}
	for id, v := range secrets {
		blob := fmt.Sprint(v)
		if strings.Contains(strings.ToUpper(blob), "CANARY") {
			t.Fatalf("canary in secret %s: %v", id, v)
		}
		m := v.(map[string]any)
		if _, ok := m["value"]; ok {
			t.Fatal("secret must not have value")
		}
		if m["delivery"] != "file" {
			t.Fatalf("delivery=%v", m["delivery"])
		}
	}
	example := importpropose.EnvExamplePlaceholders(applied, suggestionIDsForTest(res.SecretSuggestions))
	if strings.Contains(strings.ToUpper(example), "CANARY") {
		t.Fatalf("canary in env example: %s", example)
	}
	if !strings.Contains(example, "PUBLIC_HOST=") {
		t.Fatalf("missing parameter placeholder: %s", example)
	}
	if strings.Contains(example, "SECRET_CANARY_VALUE_DO_NOT_LEAK") {
		t.Fatal("raw canary leaked")
	}
}

func TestProposeMultiComposeFirstWins(t *testing.T) {
	ws := repoFixture(t, "multi")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Composes:  []string{"stack-a.yml", "stack-b.yml"},
		Name:      "multi",
	})
	if !res.OK {
		t.Fatalf("expected ok, diags=%v", res.Diagnostics)
	}
	if res.Summary.Services != 3 {
		t.Fatalf("services=%d want 3 (api,web,worker)", res.Summary.Services)
	}
	spec := res.Project["spec"].(map[string]any)
	svcs := spec["services"].(map[string]any)
	if _, ok := svcs["worker"]; !ok {
		t.Fatal("missing worker from second file")
	}
	imgs := spec["images"].(map[string]any)
	apiImg := imgs["api"].(map[string]any)
	if apiImg["ref"] != "docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001" {
		t.Fatalf("api image should keep first file ref: %v", apiImg)
	}
	foundCollision := false
	for _, r := range res.Refusals {
		if r.Category == "collision" && r.Code == diagnostic.CodeImportCollision {
			foundCollision = true
			break
		}
	}
	if !foundCollision {
		t.Fatalf("expected collision refusal, refusals=%+v diags=%v", res.Refusals, res.Diagnostics)
	}
}

func TestProposePathTraversal(t *testing.T) {
	ws := repoFixture(t, "happy")
	res := importpropose.Propose(importpropose.Request{Workspace: ws, Compose: "../etc/passwd"})
	if res.OK {
		t.Fatal("expected fail")
	}
	assertCode(t, res.Diagnostics, diagnostic.CodeImportPathInvalid)
}

func suggestionIDsForTest(sugs []importpropose.SecretSuggestion) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range sugs {
		if _, ok := seen[s.ID]; ok {
			continue
		}
		seen[s.ID] = struct{}{}
		out = append(out, s.ID)
	}
	return out
}

func assertNoCanary(t *testing.T, res importpropose.Result) {
	t.Helper()
	blob := fmt.Sprintf("%#v\n%s", res, res.EnvExample)
	for _, d := range res.Diagnostics {
		blob += d.Code + d.Message + d.Hint
	}
	if strings.Contains(blob, "SECRET_CANARY_VALUE_DO_NOT_LEAK") ||
		strings.Contains(strings.ToUpper(blob), "SUPER_SECRET_CANARY") {
		t.Fatal("secret canary leaked into propose result/diagnostics")
	}
}

func TestProposeStack4FlowCompose(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "ui", "two-service")
	if _, err := os.Stat(filepath.Join(ws, "stack4.yml")); err != nil {
		t.Skip("stack4.yml fixture not in workspace")
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "stack4.yml",
		Name:      "stack4",
	})
	for _, d := range res.Diagnostics {
		if d.Code == "MANIFEST.PARSE_ERROR" {
			t.Fatalf("compose parse error: %s", d.Message)
		}
	}
	if res.Summary.Services < 3 {
		t.Fatalf("expected at least 3 services, got %+v ok=%v", res.Summary, res.OK)
	}
	if res.Summary.Networks != 2 {
		t.Fatalf("networks=%d", res.Summary.Networks)
	}
	if res.Summary.Resources < 1 {
		t.Fatalf("expected bind resources from ../ paths, resources=%d", res.Summary.Resources)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	tika := services["tika"].(map[string]any)
	if _, ok := tika["ports"]; !ok {
		t.Fatal("tika missing ports from short syntax")
	}
}

func TestProposeStack1PortBindNotTruncated(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "ui", "two-service")
	if _, err := os.Stat(filepath.Join(ws, "stack1.yml")); err != nil {
		t.Skip("stack1.yml fixture not in workspace")
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "stack1.yml",
		Name:      "stack1",
	})
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	chroma := services["chroma"].(map[string]any)
	ports := chroma["ports"].([]any)
	if len(ports) == 0 {
		t.Fatal("no chroma ports")
	}
	p0 := ports[0].(map[string]any)
	bind, _ := p0["bind"].(string)
	if bind != "127.0.0.1" {
		t.Fatalf("bind=%q want 127.0.0.1", bind)
	}
}

func TestProposeStack1QdrantImageDefault(t *testing.T) {
	ws := filepath.Join("..", "..", "..", "testdata", "ui", "two-service")
	if _, err := os.Stat(filepath.Join(ws, "stack1.yml")); err != nil {
		t.Skip("stack1.yml fixture not in workspace")
	}
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "stack1.yml",
		Name:      "stack1",
	})
	for _, d := range res.Diagnostics {
		if d.Code == "MANIFEST.PARSE_ERROR" {
			t.Fatalf("compose parse error: %s", d.Message)
		}
		if d.Code == "IMPORT.INTERPOLATION" && d.Severity == diagnostic.SeverityError {
			t.Fatalf("unexpected interpolation error: %s", d.Message)
		}
	}
	if res.Summary.Services < 7 {
		t.Fatalf("expected 7 services (incl. qdrant), got %+v", res.Summary)
	}
	spec := res.Project["spec"].(map[string]any)
	images := spec["images"].(map[string]any)
	if _, ok := images["qdrant"]; !ok {
		t.Fatal("missing qdrant image")
	}
}

func TestProposeShortPortAndVolumeRelativeCompose(t *testing.T) {
	ws := repoFixture(t, "relpath")
	res := importpropose.Propose(importpropose.Request{
		Workspace: ws,
		Compose:   "docker/stack/compose.yaml",
		Name:      "relpath",
	})
	if !res.OK {
		t.Fatalf("expected ok, diags=%v", res.Diagnostics)
	}
	if res.Summary.Resources != 1 {
		t.Fatalf("resources=%d want 1 bind from ../data/logs", res.Summary.Resources)
	}
	spec := res.Project["spec"].(map[string]any)
	web := spec["services"].(map[string]any)["web"].(map[string]any)
	if _, ok := web["ports"]; !ok {
		t.Fatal("missing ports")
	}
	if _, ok := web["mounts"]; !ok {
		t.Fatal("missing mounts")
	}
	resources := spec["resources"].(map[string]any)
	var src string
	for _, r := range resources {
		m := r.(map[string]any)
		src, _ = m["source"].(string)
	}
	if src != "docker/data/logs" {
		t.Fatalf("resolved bind source=%q", src)
	}
}

func assertCode(t *testing.T, diags []diagnostic.Diagnostic, code string) {
	t.Helper()
	for _, d := range diags {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("missing code %s in %#v", code, diags)
}
