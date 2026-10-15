package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
)

func TestRenderEnvironmentFromOverridesKeys(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"app": {Ref: ref}},
		Services: map[string]ir.Service{
			"api": {
				Image: "app",
				Environment: map[string]ir.ValueSource{
					"LLAMA_MODEL": {Literal: "m.gguf"},
					"PORT":        {Literal: "8080"},
				},
			},
			"beat": {
				Image:           "app",
				EnvironmentFrom: "api",
				Environment: map[string]ir.ValueSource{
					"PORT": {Literal: "8090"},
				},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{
		Lock: lockfile.Document{Images: map[string]lockfile.ImageLock{"app": {Ref: ref}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	beat := text[strings.Index(text, "beat:"):]
	if !strings.Contains(beat, "LLAMA_MODEL") || !strings.Contains(beat, "m.gguf") {
		t.Fatalf("beat did not inherit LLAMA_MODEL:\n%s", text)
	}
	if !strings.Contains(beat, "8090") {
		t.Fatalf("beat did not override PORT:\n%s", text)
	}
}
