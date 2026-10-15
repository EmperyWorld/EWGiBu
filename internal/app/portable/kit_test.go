package portable

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckKitListsEmptyPathsAndMissingImages(t *testing.T) {
	dir := t.TempDir()
	hosts := []map[string]any{{
		"service": "api", "target": "/models", "parameter": "models-path", "value": "",
	}}
	raw, _ := json.Marshal(hosts)
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images.txt"), []byte("localhost/eliteew-api:dev\tload\timages/x.tar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diags := CheckKit(dir)
	var sawHost, sawImg bool
	for _, d := range diags {
		if d.Code == CodeHostPath {
			sawHost = true
		}
		if d.Code == CodeLocalImage {
			sawImg = true
		}
	}
	if !sawHost || !sawImg {
		t.Fatalf("diags=%v", diags)
	}
}

func TestCheckKitEmptyValueWithLiteralStillStops(t *testing.T) {
	dir := t.TempDir()
	hosts := []map[string]any{{
		"service": "api", "target": "/models", "parameter": "host-api",
		"literal": "/data/models", "value": "",
	}}
	raw, _ := json.Marshal(hosts)
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "images.txt"), []byte("localhost/eliteew-api:dev\tload\timages/x.tar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := imagePresent
	imagePresent = func(string) bool { return false }
	defer func() { imagePresent = prev }()
	diags := CheckKit(dir)
	var sawHost, sawImg bool
	for _, d := range diags {
		if d.Code == CodeHostPath && strings.Contains(d.Message, "host-api") {
			sawHost = true
		}
		if d.Code == CodeLocalImage && strings.Contains(d.Message, "localhost/eliteew-api:dev") {
			sawImg = true
		}
	}
	if !sawHost || !sawImg {
		t.Fatalf("diags=%v", diags)
	}
	imagePresent = func(string) bool { return true }
	hosts[0]["value"] = "/srv/models"
	raw, _ = json.Marshal(hosts)
	if err := os.WriteFile(filepath.Join(dir, "host-paths.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	diags = CheckKit(dir)
	for _, d := range diags {
		if d.Code == CodeHostPath || d.Code == CodeLocalImage {
			t.Fatalf("check should pass: %v", diags)
		}
	}
}

func TestCheckKitPassesFilledPathAndPresentImage(t *testing.T) {
	dir := t.TempDir()
	hosts := []map[string]any{{
		"service": "api", "target": "/models", "parameter": "models-path", "value": "/data/models",
	}}
	raw, _ := json.Marshal(hosts)
	_ = os.WriteFile(filepath.Join(dir, "host-paths.json"), append(raw, '\n'), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "images.txt"), []byte("docker.io/library/nginx@sha256:0000000000000000000000000000000000000000000000000000000000000001\tpull\n"), 0o644)
	diags := CheckKit(dir)
	for _, d := range diags {
		if d.Code == CodeHostPath || d.Code == CodeLocalImage {
			t.Fatalf("unexpected %v", diags)
		}
	}
}
