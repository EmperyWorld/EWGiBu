package importpropose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestProposeEscapedHealthcheckKeepsHealthyDependency(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"services:",
		"  api:",
		"    image: docker.io/library/nginx:1.27",
		"    depends_on:",
		"      db:",
		"        condition: service_healthy",
		"  db:",
		"    image: docker.io/library/postgres:16",
		"    healthcheck:",
		"      test: ['CMD-SHELL', 'pg_isready -U \"$${POSTGRES_USER}\" -d \"$${POSTGRES_DB}\"']",
		"      interval: 10s",
		"      timeout: 5s",
		"      retries: 5",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "olenti"})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	db := services["db"].(map[string]any)
	hc, _ := db["healthcheck"].(map[string]any)
	cmd, _ := hc["command"].([]any)
	const script = `pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"`
	if len(cmd) != 3 || cmd[0] != "/bin/sh" || cmd[1] != "-c" || cmd[2] != script {
		t.Fatalf("healthcheck=%#v", hc["command"])
	}
	api := services["api"].(map[string]any)
	deps, _ := api["dependsOn"].([]any)
	dep, _ := deps[0].(map[string]any)
	if dep["target"] != "db" || dep["condition"] != "healthy" {
		t.Fatalf("dependsOn=%v", api["dependsOn"])
	}
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, "healthcheck omitted") {
			t.Fatalf("probe omitted: %s", d.Message)
		}
	}
	assertDraftDecodes(t, res.Project)
}

func TestProposeHealthyDowngradedWhenHealthcheckOmitted(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"services:",
		"  api:",
		"    image: docker.io/library/nginx:1.27",
		"    depends_on:",
		"      db:",
		"        condition: service_healthy",
		"  db:",
		"    image: docker.io/library/postgres:16",
		"    healthcheck:",
		"      test: ['CMD-SHELL', 'pg_isready -U \"${POSTGRES_USER}\"']",
		"      interval: 10s",
		"      timeout: 5s",
		"      retries: 5",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Propose(Request{Workspace: dir, Compose: "compose.yaml", Name: "olenti"})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	db := services["db"].(map[string]any)
	if _, ok := db["healthcheck"]; ok {
		t.Fatal("unescaped interpolation must not become a healthcheck")
	}
	api := services["api"].(map[string]any)
	deps, _ := api["dependsOn"].([]any)
	dep, _ := deps[0].(map[string]any)
	if dep["condition"] != "started" {
		t.Fatalf("dependsOn=%v", api["dependsOn"])
	}
	var downgraded bool
	for _, d := range res.Diagnostics {
		if d.Severity == diagnostic.SeverityError {
			t.Fatalf("error %s %s", d.Code, d.Message)
		}
		if strings.Contains(d.Message, "downgraded to started") {
			downgraded = true
		}
	}
	if !downgraded {
		t.Fatal("expected downgrade warning")
	}
	assertDraftDecodes(t, res.Project)
}

func assertDraftDecodes(t *testing.T, project map[string]any) {
	t.Helper()
	raw, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	doc := loader.Parse("import-draft.json", raw)
	if _, diags := v1alpha1.Decode(doc); diagnostic.HasError(diags) {
		t.Fatalf("draft invalid: %v", diags)
	}
}
