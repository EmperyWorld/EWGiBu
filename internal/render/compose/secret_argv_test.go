package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
	"iacpack/internal/runtime/secrets"
)

func TestRenderSecretArgvAndEnvParts(t *testing.T) {
	ref := "docker.io/library/redis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"redis": {Ref: ref}},
		Services: map[string]ir.Service{
			"redis": {
				Image: "redis",
				User:  "0:0",
				Command: []ir.ArgvElement{
					{Literal: "redis-server"},
					{Literal: "--requirepass"},
					{Secret: "redis-password"},
				},
				Environment: map[string]ir.ValueSource{
					"PAPERLESS_REDIS": {
						Parts: []ir.ValueSource{
							{Literal: "redis://:"},
							{Secret: "redis-password"},
							{Literal: "@redis:6379/1"},
						},
					},
				},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{Lock: lockfile.Document{
		Images: map[string]lockfile.ImageLock{"redis": {Ref: ref}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := f.Services["redis"].Command
	ph := secrets.ComposePlaceholder("redis-password")
	if len(cmd) != 3 || cmd[1] != "--requirepass" || cmd[2] != ph {
		t.Fatalf("command=%#v", cmd)
	}
	if f.Services["redis"].User != "0:0" {
		t.Fatal(f.Services["redis"].User)
	}
	env := f.Services["redis"].Environment["PAPERLESS_REDIS"]
	if env != "redis://:"+ph+"@redis:6379/1" {
		t.Fatalf("env=%q", env)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "user: \"0:0\"") && !strings.Contains(text, "user: 0:0") {
		t.Fatalf("missing user:\n%s", text)
	}
	if !strings.Contains(text, ph) {
		t.Fatalf("missing secret placeholder:\n%s", text)
	}
}
