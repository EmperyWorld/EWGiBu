package importpropose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/diagnostic"
)

func TestLinkEnvironmentFromKeepsOverrides(t *testing.T) {
	ref := "docker.io/library/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	images := map[string]any{
		"api":  map[string]any{"ref": ref, "delivery": "pull"},
		"beat": map[string]any{"ref": ref, "delivery": "pull"},
	}
	services := map[string]any{
		"api": map[string]any{
			"image": "api",
			"environment": map[string]any{
				"LLAMA_MODEL":      map[string]any{"literal": "m.gguf"},
				"PORT":             map[string]any{"literal": "8080"},
				"ELITEEW_LOG_FILE": map[string]any{"literal": "/logs/api.log"},
			},
		},
		"beat": map[string]any{
			"image": "beat",
			"environment": map[string]any{
				"LLAMA_MODEL":      map[string]any{"literal": "m.gguf"},
				"PORT":             map[string]any{"literal": "8080"},
				"ELITEEW_LOG_FILE": map[string]any{"literal": "/logs/beat.log"},
			},
		},
	}
	var diags []diagnostic.Diagnostic
	linkEnvironmentFrom(services, images, &diags)
	beat := services["beat"].(map[string]any)
	if beat["environmentFrom"] != "api" {
		t.Fatalf("environmentFrom=%v", beat["environmentFrom"])
	}
	env := beat["environment"].(map[string]any)
	if len(env) != 1 {
		t.Fatalf("overrides=%v", env)
	}
	log := env["ELITEEW_LOG_FILE"].(map[string]any)
	if log["literal"] != "/logs/beat.log" {
		t.Fatalf("log override=%v", log)
	}
	if _, ok := env["LLAMA_MODEL"]; ok {
		t.Fatal("inherited key should not remain")
	}
}

func TestApplyImageUsersWritesNumericConfigUser(t *testing.T) {
	prev := imageConfigUser
	imageConfigUser = func(ref string) (string, bool) { return "1000", true }
	defer func() { imageConfigUser = prev }()
	services := map[string]any{"keycloak": map[string]any{"image": "keycloak"}}
	images := map[string]any{"keycloak": map[string]any{"ref": "quay.io/keycloak/keycloak:26"}}
	var diags []diagnostic.Diagnostic
	applyImageUsers(services, images, "", nil, &diags)
	if services["keycloak"].(map[string]any)["user"] != "1000" {
		t.Fatalf("%v", services["keycloak"])
	}
}

func TestApplyImageUsersReportsName(t *testing.T) {
	prev := imageConfigUser
	imageConfigUser = func(ref string) (string, bool) { return "keycloak", true }
	defer func() { imageConfigUser = prev }()
	services := map[string]any{"keycloak": map[string]any{"image": "keycloak"}}
	images := map[string]any{"keycloak": map[string]any{"ref": "quay.io/keycloak/keycloak:26"}}
	var diags []diagnostic.Diagnostic
	applyImageUsers(services, images, "", nil, &diags)
	if _, ok := services["keycloak"].(map[string]any)["user"]; ok {
		t.Fatal("name must not become user")
	}
	found := false
	for _, d := range diags {
		if d.Code == diagnostic.CodeImportUserName {
			found = true
		}
	}
	if !found {
		t.Fatalf("diags=%v", diags)
	}
}

func TestApplyImageUsersKeepsExistingDraft(t *testing.T) {
	services := map[string]any{
		"keycloak": map[string]any{"image": "keycloak"},
	}
	images := map[string]any{
		"keycloak": map[string]any{"ref": "quay.io/keycloak/keycloak@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}
	existing := map[string]any{
		"spec": map[string]any{
			"services": map[string]any{
				"keycloak": map[string]any{"user": "1000"},
			},
		},
	}
	var diags []diagnostic.Diagnostic
	applyImageUsers(services, images, "", existing, &diags)
	if services["keycloak"].(map[string]any)["user"] != "1000" {
		t.Fatalf("%v", services["keycloak"])
	}
}

func TestApplyImageUsersReportsAbsentImage(t *testing.T) {
	prev := imageConfigUser
	imageConfigUser = func(string) (string, bool) { return "", false }
	defer func() { imageConfigUser = prev }()
	services := map[string]any{"tika": map[string]any{"image": "tika"}}
	images := map[string]any{"tika": map[string]any{"ref": "docker.io/apache/tika:latest"}}
	var diags []diagnostic.Diagnostic
	applyImageUsers(services, images, "", nil, &diags)
	if _, ok := services["tika"].(map[string]any)["user"]; ok {
		t.Fatal("missing image must not invent user")
	}
	found := false
	for _, d := range diags {
		if d.Code == diagnostic.CodeImportUserAbsent && strings.Contains(d.Message, "tika") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diags=%v", diags)
	}
}

func TestNumericImportUser(t *testing.T) {
	if !numericImportUser("1000") || !numericImportUser("0:0") || !numericImportUser("0") {
		t.Fatal("numeric rejected")
	}
	if numericImportUser("keycloak") || numericImportUser("nginx") {
		t.Fatal("name accepted")
	}
}

func TestDockerfileUserWhenImageMissing(t *testing.T) {
	prev := imageConfigUser
	imageConfigUser = func(string) (string, bool) { return "", false }
	defer func() { imageConfigUser = prev }()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	df := "FROM scratch\nUSER nginx\nUSER ${APP_UID:-1000}\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "Dockerfile"), []byte(df), 0o644); err != nil {
		t.Fatal(err)
	}
	services := map[string]any{"api": map[string]any{"image": "api"}}
	images := map[string]any{"api": map[string]any{
		"ref": "localhost/api:dev",
		"build": map[string]any{
			"context":    "app",
			"dockerfile": "Dockerfile",
		},
	}}
	var diags []diagnostic.Diagnostic
	applyImageUsers(services, images, dir, nil, &diags)
	if services["api"].(map[string]any)["user"] != "1000" {
		t.Fatalf("%v diags=%v", services["api"], diags)
	}
}
