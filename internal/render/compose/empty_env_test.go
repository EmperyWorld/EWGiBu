package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
)

func TestRenderOmitsEmptyStringEnvParams(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Parameters: map[string]ir.Parameter{
			"api-host": {Phase: "deploy", Type: "string"},
			"workers":  {Phase: "deploy", Type: "integer"},
		},
		Images: map[string]ir.Image{"app": {Ref: ref}},
		Services: map[string]ir.Service{
			"api": {
				Image: "app",
				Environment: map[string]ir.ValueSource{
					"API_HOST": {Parameter: "api-host"},
					"WORKERS":  {Parameter: "workers"},
					"FIXED":    {Literal: "ok"},
				},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		DeployValues: map[string]string{"api-host": "", "workers": "2"},
		Lock: lockfile.Document{Images: map[string]lockfile.ImageLock{
			"app": {Ref: ref},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	env := f.Services["api"].Environment
	if _, ok := env["API_HOST"]; ok {
		t.Fatalf("empty string param must be omitted: %#v", env)
	}
	if env["WORKERS"] != "2" || env["FIXED"] != "ok" {
		t.Fatalf("%#v", env)
	}
}

func TestRenderRejectsEmptyTypedEnvParams(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Parameters: map[string]ir.Parameter{
			"workers": {Phase: "deploy", Type: "integer"},
		},
		Images: map[string]ir.Image{"app": {Ref: ref}},
		Services: map[string]ir.Service{
			"api": {
				Image: "app",
				Environment: map[string]ir.ValueSource{
					"WORKERS": {Parameter: "workers"},
				},
			},
		},
	}
	_, err := compose.Render(d, compose.RenderOptions{
		DeployValues: map[string]string{"workers": ""},
		Lock: lockfile.Document{Images: map[string]lockfile.ImageLock{
			"app": {Ref: ref},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "workers") {
		t.Fatalf("got %v", err)
	}
}
