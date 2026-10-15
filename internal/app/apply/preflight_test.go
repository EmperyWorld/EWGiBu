package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/runtime/secrets"
)

func TestNetworkCreateNameUsesExternal(t *testing.T) {
	dep := ir.Deployment{Networks: map[string]ir.Network{
		"ew-net-2847": {External: &ir.ExternalRef{Name: "ew_net_2847"}},
		"backend":     {},
	}}
	if networkCreateName(dep, "ew-net-2847") != "ew_net_2847" {
		t.Fatal(networkCreateName(dep, "ew-net-2847"))
	}
	if networkCreateName(dep, "backend") != "backend" {
		t.Fatal(networkCreateName(dep, "backend"))
	}
}

func TestPreflightSeparatesPullLocalHostAndParams(t *testing.T) {
	dir := t.TempDir()
	models := filepath.Join(dir, "models")
	if err := os.Mkdir(models, 0o755); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dir, "db-password")
	if err := os.WriteFile(secretPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dep := ir.Deployment{
		Parameters: map[string]ir.Parameter{
			"stack2-llama-models-path": {Phase: "deploy", Type: "string"},
			"api-port":                 {Phase: "deploy", Type: "string"},
			"filled":                   {Phase: "deploy", Type: "string", Default: "1"},
		},
		Secrets: map[string]ir.Secret{
			"db-password": {Required: true},
		},
		Images: map[string]ir.Image{
			"api":   {Ref: "localhost/eliteew-api:prod-baked-20260928"},
			"front": {Ref: "eliteew-frontend-fedora:latest"},
			"pg":    {Ref: "docker.io/library/postgres:18.4"},
			"hub":   {Ref: "apache/tika:3.3.1.0"},
		},
		Services: map[string]ir.Service{
			"api":   {Image: "api"},
			"front": {Image: "front"},
			"pg":    {Image: "pg"},
			"hub":   {Image: "hub"},
			"llama": {
				Image: "pg",
				Environment: map[string]ir.ValueSource{
					"PORT": {Parameter: "api-port"},
				},
				Mounts: []ir.Mount{{
					HostParameter: "stack2-llama-models-path",
					Target:        "/models",
				}},
			},
		},
	}
	deploy := map[string]string{
		"stack2-llama-models-path": filepath.Join(dir, "missing-models"),
		"filled":                   "1",
	}
	present := func(ref string) (bool, bool) {
		return false, true
	}
	diags := preflightDeployment(dep, deploy, false, []secrets.Ref{{ID: "db-password", Path: secretPath}}, present)
	var sawPull, sawLocal, sawHost, sawParam bool
	for _, d := range diags {
		switch d.Code {
		case codeImagePull:
			sawPull = strings.Contains(d.Hint, "podman pull ") &&
				strings.Contains(d.Hint, "docker.io/library/postgres:18.4") &&
				strings.Contains(d.Hint, "apache/tika:3.3.1.0") &&
				!strings.Contains(d.Hint, ",")
		case codeImageLocal:
			sawLocal = strings.Contains(d.Message, "localhost/eliteew-api:prod-baked-20260928") && strings.Contains(d.Message, "eliteew-frontend-fedora:latest")
		case codeHostPath:
			sawHost = strings.Contains(d.Message, "stack2-llama-models-path")
		case "PARAM.MISSING":
			sawParam = strings.Contains(d.Message, "api-port") && !strings.Contains(d.Message, "filled")
		}
		if strings.Contains(d.Message, "nope") {
			t.Fatal("unexpected secret material")
		}
	}
	if !sawPull || !sawLocal || !sawHost || !sawParam {
		t.Fatalf("pull=%v local=%v host=%v param=%v diags=%v", sawPull, sawLocal, sawHost, sawParam, diags)
	}
	// existing directory is not reported
	deploy["stack2-llama-models-path"] = models
	diags = preflightDeployment(dep, deploy, false, []secrets.Ref{{ID: "db-password", Path: secretPath}}, func(string) (bool, bool) { return true, true })
	for _, d := range diags {
		if d.Code == codeHostPath || d.Code == codeImageLocal || d.Code == codeImagePull {
			t.Fatalf("present images and existing path should be quiet: %+v", d)
		}
	}
}
