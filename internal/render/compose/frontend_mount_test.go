package compose_test

import (
	"strings"
	"testing"

	"iacpack/internal/domain/ir"
	"iacpack/internal/lockfile"
	"iacpack/internal/render/compose"
)

func TestRenderEmptyArgvLiteral(t *testing.T) {
	ref := "docker.io/library/redis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"redis": {Ref: ref}},
		Services: map[string]ir.Service{
			"redis": {
				Image: "redis",
				Command: []ir.ArgvElement{
					{Literal: "redis-server"},
					{Literal: "--logfile"},
					{Literal: ""},
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
	if len(cmd) != 3 || cmd[1] != "--logfile" || cmd[2] != "" {
		t.Fatalf("command=%#v", cmd)
	}
	raw, err := compose.MarshalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "- \"\"") {
		t.Fatalf("empty arg not quoted:\n%s", raw)
	}
}

func TestRenderBuildMount(t *testing.T) {
	ref := "docker.io/library/nginx@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	d := ir.Deployment{
		Project: ir.ProjectRef{Name: "demo"},
		Images:  map[string]ir.Image{"web": {Ref: ref}},
		Services: map[string]ir.Service{
			"frontend": {
				Image: "web",
				Mounts: []ir.Mount{{
					Build: "spa", Target: "/usr/share/nginx/html", ReadOnly: true,
				}},
			},
		},
	}
	f, err := compose.Render(d, compose.RenderOptions{Lock: lockfile.Document{
		Images: map[string]lockfile.ImageLock{"web": {Ref: ref}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	vols := f.Services["frontend"].Volumes
	if len(vols) != 1 || vols[0].Source != "./payload/build-spa" || vols[0].Target != "/usr/share/nginx/html" || !vols[0].ReadOnly {
		t.Fatalf("mount=%#v", vols)
	}
}
