package importpropose_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/app/importpropose"
	"iacpack/internal/diagnostic"
	"iacpack/internal/manifest/loader"
	"iacpack/internal/manifest/v1alpha1"
)

func TestProposeSecretEnvUsesInterpolationID(t *testing.T) {
	dir := t.TempDir()
	body := strings.Join([]string{
		"services:",
		"  rabbitmq:",
		"    image: docker.io/library/rabbitmq:3",
		"    environment:",
		"      RABBITMQ_DEFAULT_PASS: ${RABBITMQ_PASSWORD}",
		"      RABBITMQ_PASSWORD: ${RABBITMQ_PASSWORD}",
		"  postgres:",
		"    image: docker.io/library/postgres:16",
		"    environment:",
		"      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}",
		"  keycloak-postgres:",
		"    image: docker.io/library/postgres:16",
		"    environment:",
		"      POSTGRES_PASSWORD: ${KEYCLOAK_DB_PASSWORD:?required}",
		"  keycloak:",
		"    image: quay.io/keycloak/keycloak:26",
		"    environment:",
		"      KC_DB_PASSWORD: ${KEYCLOAK_DB_PASSWORD:?required}",
		"      JITSI_TOKEN_TTL_SECONDS: ${JITSI_TOKEN_TTL_SECONDS:-300}",
		"    env_file:",
		"      - ../env_file/.env.stack0.shared",
		"      - ../env_file/.env.stack0.api",
		"      - ../env_file/.env.stack0.jobs",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res := importpropose.Propose(importpropose.Request{Workspace: dir, Compose: "compose.yaml", Name: "eliteew"})
	if !res.OK {
		t.Fatalf("ok=false diags=%v", res.Diagnostics)
	}
	spec := res.Project["spec"].(map[string]any)
	services := spec["services"].(map[string]any)
	pg := services["postgres"].(map[string]any)["environment"].(map[string]any)["POSTGRES_PASSWORD"].(map[string]any)
	if pg["secret"] != "postgres-password" {
		t.Fatalf("postgres secret=%v", pg)
	}
	kcPG := services["keycloak-postgres"].(map[string]any)["environment"].(map[string]any)["POSTGRES_PASSWORD"].(map[string]any)
	if kcPG["secret"] != "keycloak-db-password" {
		t.Fatalf("keycloak postgres secret=%v", kcPG)
	}
	kc := services["keycloak"].(map[string]any)["environment"].(map[string]any)
	if kc["KC_DB_PASSWORD"].(map[string]any)["secret"] != "keycloak-db-password" {
		t.Fatalf("kc db=%v", kc["KC_DB_PASSWORD"])
	}
	ttl := kc["JITSI_TOKEN_TTL_SECONDS"].(map[string]any)
	if _, isSecret := ttl["secret"]; isSecret {
		t.Fatalf("ttl must stay a parameter, got %v", ttl)
	}
	if ttl["parameter"] != "jitsi-token-ttl-seconds" {
		t.Fatalf("ttl=%v", ttl)
	}
	rmq := services["rabbitmq"].(map[string]any)["environment"].(map[string]any)
	if rmq["RABBITMQ_DEFAULT_PASS"].(map[string]any)["secret"] != "rabbitmq-password" {
		t.Fatalf("RABBITMQ_DEFAULT_PASS=%v", rmq["RABBITMQ_DEFAULT_PASS"])
	}
	if rmq["RABBITMQ_PASSWORD"].(map[string]any)["secret"] != "rabbitmq-password" {
		t.Fatalf("RABBITMQ_PASSWORD=%v", rmq["RABBITMQ_PASSWORD"])
	}
	secrets := spec["secrets"].(map[string]any)
	for _, id := range []string{"postgres-password", "keycloak-db-password", "rabbitmq-password", "env-file-env-stack0-shared", "env-file-env-stack0-api", "env-file-env-stack0-jobs"} {
		sec, ok := secrets[id].(map[string]any)
		if !ok {
			t.Fatalf("missing secret %s in %v", id, secrets)
		}
		if sec["delivery"] != "file" {
			t.Fatalf("%s delivery=%v", id, sec["delivery"])
		}
		if _, hasValue := sec["value"]; hasValue {
			t.Fatal("secret value stored")
		}
	}
	raw, err := json.Marshal(res.Project)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "required}") || strings.Contains(strings.ToLower(string(raw)), "hunter2") {
		t.Fatal("interpolation text leaked into the draft")
	}
	doc := loader.Parse("import-draft.json", raw)
	if _, diags := v1alpha1.Decode(doc); diagnostic.HasError(diags) {
		t.Fatalf("draft invalid: %v", diags)
	}
}
